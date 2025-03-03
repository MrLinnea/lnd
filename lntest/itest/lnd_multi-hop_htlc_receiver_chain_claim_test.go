package itest

import (
	"context"
	"time"

	"github.com/ltcsuite/lnd/lncfg"
	"github.com/ltcsuite/lnd/lnrpc"
	"github.com/ltcsuite/lnd/lnrpc/invoicesrpc"
	"github.com/ltcsuite/lnd/lnrpc/routerrpc"
	"github.com/ltcsuite/lnd/lntest"
	"github.com/ltcsuite/lnd/lntest/wait"
	"github.com/ltcsuite/lnd/lntypes"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/stretchr/testify/require"
)

// testMultiHopReceiverChainClaim tests that in the multi-hop setting, if the
// receiver of an HTLC knows the preimage, but wasn't able to settle the HTLC
// off-chain, then it goes on chain to claim the HTLC uing the HTLC success
// transaction. In this scenario, the node that sent the outgoing HTLC should
// extract the preimage from the sweep transaction, and finish settling the
// HTLC backwards into the route.
func testMultiHopReceiverChainClaim(net *lntest.NetworkHarness, t *harnessTest,
	alice, bob *lntest.HarnessNode, c lnrpc.CommitmentType) {

	ctxb := context.Background()

	// First, we'll create a three hop network: Alice -> Bob -> Carol, with
	// Carol refusing to actually settle or directly cancel any HTLC's
	// self.
	aliceChanPoint, bobChanPoint, carol := createThreeHopNetwork(
		t, net, alice, bob, false, c,
	)

	// Clean up carol's node when the test finishes.
	defer shutdownAndAssert(net, t, carol)

	// With the network active, we'll now add a new hodl invoice at Carol's
	// end. Make sure the cltv expiry delta is large enough, otherwise Bob
	// won't send out the outgoing htlc.

	const invoiceAmt = 100000
	preimage := lntypes.Preimage{1, 2, 4}
	payHash := preimage.Hash()
	invoiceReq := &invoicesrpc.AddHoldInvoiceRequest{
		Value:      invoiceAmt,
		CltvExpiry: 40,
		Hash:       payHash[:],
	}
	ctxt, cancel := context.WithTimeout(ctxb, defaultTimeout)
	defer cancel()
	carolInvoice, err := carol.AddHoldInvoice(ctxt, invoiceReq)
	require.NoError(t.t, err)

	// Now that we've created the invoice, we'll send a single payment from
	// Alice to Carol. We won't wait for the response however, as Carol
	// will not immediately settle the payment.
	ctx, cancel := context.WithCancel(ctxb)
	defer cancel()

	_, err = alice.RouterClient.SendPaymentV2(
		ctx, &routerrpc.SendPaymentRequest{
			PaymentRequest: carolInvoice.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)
	require.NoError(t.t, err)

	// At this point, all 3 nodes should now have an active channel with
	// the created HTLC pending on all of them.
	nodes := []*lntest.HarnessNode{alice, bob, carol}
	err = wait.NoError(func() error {
		return assertActiveHtlcs(nodes, payHash[:])
	}, defaultTimeout)
	require.NoError(t.t, err)

	// Wait for carol to mark invoice as accepted. There is a small gap to
	// bridge between adding the htlc to the channel and executing the exit
	// hop logic.
	waitForInvoiceAccepted(t, carol, payHash)

	restartBob, err := net.SuspendNode(bob)
	require.NoError(t.t, err)

	// Settle invoice. This will just mark the invoice as settled, as there
	// is no link anymore to remove the htlc from the commitment tx. For
	// this test, it is important to actually settle and not leave the
	// invoice in the accepted state, because without a known preimage, the
	// channel arbitrator won't go to chain.
	ctx, cancel = context.WithTimeout(ctxb, defaultTimeout)
	defer cancel()
	_, err = carol.SettleInvoice(ctx, &invoicesrpc.SettleInvoiceMsg{
		Preimage: preimage[:],
	})
	require.NoError(t.t, err)

	// Increase the fee estimate so that the following force close tx will
	// be cpfp'ed.
	net.SetFeeEstimate(30000)

	// Now we'll mine enough blocks to prompt carol to actually go to the
	// chain in order to sweep her HTLC since the value is high enough.
	// TODO(roasbeef): modify once go to chain policy changes
	numBlocks := padCLTV(uint32(
		invoiceReq.CltvExpiry - lncfg.DefaultIncomingBroadcastDelta,
	))
	_, err = net.Miner.Client.Generate(numBlocks)
	require.NoError(t.t, err)

	// At this point, Carol should broadcast her active commitment
	// transaction in order to go to the chain and sweep her HTLC. If there
	// are anchors, Carol also sweeps hers.
	expectedTxes := 1
	hasAnchors := commitTypeHasAnchors(c)
	if hasAnchors {
		expectedTxes = 2
	}
	_, err = getNTxsFromMempool(
		net.Miner.Client, expectedTxes, minerMempoolTimeout,
	)
	require.NoError(t.t, err)

	bobFundingTxid, err := lnrpc.GetChanPointFundingTxid(bobChanPoint)
	require.NoError(t.t, err)

	carolFundingPoint := wire.OutPoint{
		Hash:  *bobFundingTxid,
		Index: bobChanPoint.OutputIndex,
	}

	// The commitment transaction should be spending from the funding
	// transaction.
	closingTx := getSpendingTxInMempool(
		t, net.Miner.Client, minerMempoolTimeout, carolFundingPoint,
	)
	closingTxid := closingTx.TxHash()

	// Confirm the commitment.
	mineBlocks(t, net, 1, expectedTxes)

	// Restart bob again.
	err = restartBob()
	require.NoError(t.t, err)

	// After the force close transaction is mined, a series of transactions
	// should be broadcast by Bob and Carol. When Bob notices Carol's second
	// level transaction in the mempool, he will extract the preimage and
	// settle the HTLC back off-chain.
	switch c {
	// Carol should broadcast her second level HTLC transaction and Bob
	// should broadcast a sweep tx to sweep his output in the channel with
	// Carol.
	case lnrpc.CommitmentType_LEGACY:
		expectedTxes = 2

	// Carol should broadcast her second level HTLC transaction and Bob
	// should broadcast a sweep tx to sweep his output in the channel with
	// Carol, and another sweep tx to sweep his anchor output.
	case lnrpc.CommitmentType_ANCHORS:
		expectedTxes = 3

	// Carol should broadcast her second level HTLC transaction and Bob
	// should broadcast a sweep tx to sweep his anchor output. Bob's commit
	// output can't be swept yet as he's incurring an additional CLTV from
	// being the channel initiator of a script-enforced leased channel.
	case lnrpc.CommitmentType_SCRIPT_ENFORCED_LEASE:
		expectedTxes = 2

	default:
		t.Fatalf("unhandled commitment type %v", c)
	}
	txes, err := getNTxsFromMempool(
		net.Miner.Client, expectedTxes, minerMempoolTimeout,
	)
	require.NoError(t.t, err)

	// All transactions should be spending from the commitment transaction.
	assertAllTxesSpendFrom(t, txes, closingTxid)

	// We'll now mine an additional block which should confirm both the
	// second layer transactions.
	_, err = net.Miner.Client.Generate(1)
	require.NoError(t.t, err)

	time.Sleep(time.Second * 4)

	// TODO(roasbeef): assert bob pending state as well

	// Carol's pending channel report should now show two outputs under
	// limbo: her commitment output, as well as the second-layer claim
	// output.
	pendingChansRequest := &lnrpc.PendingChannelsRequest{}
	ctxt, _ = context.WithTimeout(ctxb, defaultTimeout)
	pendingChanResp, err := carol.PendingChannels(ctxt, pendingChansRequest)
	require.NoError(t.t, err)

	require.NotZero(t.t, len(pendingChanResp.PendingForceClosingChannels))
	forceCloseChan := pendingChanResp.PendingForceClosingChannels[0]
	require.NotZero(t.t, forceCloseChan.LimboBalance)

	// The pending HTLC carol has should also now be in stage 2.
	require.Len(t.t, forceCloseChan.PendingHtlcs, 1)
	require.Equal(t.t, uint32(2), forceCloseChan.PendingHtlcs[0].Stage)

	// Once the second-level transaction confirmed, Bob should have
	// extracted the preimage from the chain, and sent it back to Alice,
	// clearing the HTLC off-chain.
	nodes = []*lntest.HarnessNode{alice}
	err = wait.NoError(func() error {
		return assertNumActiveHtlcs(nodes, 0)
	}, defaultTimeout)
	require.NoError(t.t, err)

	// If we mine 4 additional blocks, then Carol can sweep the second level
	// HTLC output.
	_, err = net.Miner.Client.Generate(defaultCSV)
	require.NoError(t.t, err)

	// We should have a new transaction in the mempool.
	_, err = waitForTxInMempool(net.Miner.Client, minerMempoolTimeout)
	require.NoError(t.t, err)

	// Finally, if we mine an additional block to confirm these two sweep
	// transactions, Carol should not show a pending channel in her report
	// afterwards.
	_, err = net.Miner.Client.Generate(1)
	require.NoError(t.t, err)
	err = waitForNumChannelPendingForceClose(carol, 0, nil)
	require.NoError(t.t, err)

	// The invoice should show as settled for Carol, indicating that it was
	// swept on-chain.
	invoicesReq := &lnrpc.ListInvoiceRequest{}
	invoicesResp, err := carol.ListInvoices(ctxb, invoicesReq)
	require.NoError(t.t, err)
	require.Len(t.t, invoicesResp.Invoices, 1)
	invoice := invoicesResp.Invoices[0]
	require.Equal(t.t, lnrpc.Invoice_SETTLED, invoice.State)
	require.Equal(t.t, int64(invoiceAmt), invoice.AmtPaidSat)

	// Finally, check that the Alice's payment is correctly marked
	// succeeded.
	err = checkPaymentStatus(alice, preimage, lnrpc.Payment_SUCCEEDED)
	require.NoError(t.t, err)

	if c == lnrpc.CommitmentType_SCRIPT_ENFORCED_LEASE {
		// Bob still has his commit output to sweep to since he incurred
		// an additional CLTV from being the channel initiator of a
		// script-enforced leased channel, regardless of whether he
		// forced closed the channel or not.
		ctxt, _ = context.WithTimeout(ctxb, defaultTimeout)
		pendingChanResp, err := bob.PendingChannels(
			ctxt, &lnrpc.PendingChannelsRequest{},
		)
		require.NoError(t.t, err)

		require.Len(t.t, pendingChanResp.PendingForceClosingChannels, 1)
		forceCloseChan := pendingChanResp.PendingForceClosingChannels[0]
		require.Positive(t.t, forceCloseChan.LimboBalance)
		require.Positive(t.t, forceCloseChan.BlocksTilMaturity)

		// TODO: Bob still shows a pending HTLC at this point when he
		// shouldn't, as he already extracted the preimage from Carol's
		// claim.
		// require.Len(t.t, forceCloseChan.PendingHtlcs, 0)

		// Mine enough blocks for Bob's commit output's CLTV to expire
		// and sweep it.
		_ = mineBlocks(t, net, uint32(forceCloseChan.BlocksTilMaturity), 0)
		commitOutpoint := wire.OutPoint{Hash: closingTxid, Index: 3}
		assertSpendingTxInMempool(
			t, net.Miner.Client, minerMempoolTimeout, commitOutpoint,
		)
		_, err = net.Miner.Client.Generate(1)
		require.NoError(t.t, err)
	}

	err = waitForNumChannelPendingForceClose(bob, 0, nil)
	require.NoError(t.t, err)

	// We'll close out the channel between Alice and Bob, then shutdown
	// carol to conclude the test.
	closeChannelAndAssertType(t, net, alice, aliceChanPoint, false, false)
}
