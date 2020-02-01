package chanfunding

import (
	"fmt"

	"github.com/ltcsuite/ltcd/btcec"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcutil"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
)

// ShimIntent is an intent created by the CannedAssembler which represents a
// funding output to be created that was constructed outside the wallet. This
// might be used when a hardware wallet, or a channel factory is the entity
// crafting the funding transaction, and not lnd.
type ShimIntent struct {
	// localFundingAmt is the final amount we put into the funding output.
	localFundingAmt ltcutil.Amount

	// remoteFundingAmt is the final amount the remote party put into the
	// funding output.
	remoteFundingAmt ltcutil.Amount

	// localKey is our multi-sig key.
	localKey *keychain.KeyDescriptor

	// remoteKey is the remote party's multi-sig key.
	remoteKey *btcec.PublicKey

	// chanPoint is the final channel point for the to be created channel.
	chanPoint *wire.OutPoint
}

// FundingOutput returns the witness script, and the output that creates the
// funding output.
//
// NOTE: This method satisfies the chanfunding.Intent interface.
func (s *ShimIntent) FundingOutput() ([]byte, *wire.TxOut, error) {
	if s.localKey == nil || s.remoteKey == nil {
		return nil, nil, fmt.Errorf("unable to create witness " +
			"script, no funding keys")
	}

	totalAmt := s.localFundingAmt + s.remoteFundingAmt
	return input.GenFundingPkScript(
		s.localKey.PubKey.SerializeCompressed(),
		s.remoteKey.SerializeCompressed(),
		int64(totalAmt),
	)
}

// Cancel allows the caller to cancel a funding Intent at any time.  This will
// return any resources such as coins back to the eligible pool to be used in
// order channel fundings.
//
// NOTE: This method satisfies the chanfunding.Intent interface.
func (s *ShimIntent) Cancel() {
}

// RemoteFundingAmt is the amount the remote party put into the channel.
//
// NOTE: This method satisfies the chanfunding.Intent interface.
func (s *ShimIntent) LocalFundingAmt() ltcutil.Amount {
	return s.localFundingAmt
}

// LocalFundingAmt is the amount we put into the channel. This may differ from
// the local amount requested, as depending on coin selection, we may bleed
// from of that LocalAmt into fees to minimize change.
//
// NOTE: This method satisfies the chanfunding.Intent interface.
func (s *ShimIntent) RemoteFundingAmt() ltcutil.Amount {
	return s.remoteFundingAmt
}

// ChanPoint returns the final outpoint that will create the funding output
// described above.
//
// NOTE: This method satisfies the chanfunding.Intent interface.
func (s *ShimIntent) ChanPoint() (*wire.OutPoint, error) {
	if s.chanPoint == nil {
		return nil, fmt.Errorf("chan point unknown, funding output " +
			"not constructed")
	}

	return s.chanPoint, nil
}

// FundingKeys couples our multi-sig key along with the remote party's key.
type FundingKeys struct {
	// LocalKey is our multi-sig key.
	LocalKey *keychain.KeyDescriptor

	// RemoteKey is the multi-sig key of the remote party.
	RemoteKey *btcec.PublicKey
}

// MultiSigKeys returns the committed multi-sig keys, but only if they've been
// specified/provided.
func (s *ShimIntent) MultiSigKeys() (*FundingKeys, error) {
	if s.localKey == nil || s.remoteKey == nil {
		return nil, fmt.Errorf("unknown funding keys")
	}

	return &FundingKeys{
		LocalKey:  s.localKey,
		RemoteKey: s.remoteKey,
	}, nil
}

// A compile-time check to ensure ShimIntent adheres to the Intent interface.
var _ Intent = (*ShimIntent)(nil)

// CannedAssembler is a type of chanfunding.Assembler wherein the funding
// transaction is constructed outside of lnd, and may already exist. This
// Assembler serves as a shim which gives the funding flow the only thing it
// actually needs to proceed: the channel point.
type CannedAssembler struct {
	// fundingAmt is the total amount of coins in the funding output.
	fundingAmt ltcutil.Amount

	// localKey is our multi-sig key.
	localKey *keychain.KeyDescriptor

	// remoteKey is the remote party's multi-sig key.
	remoteKey *btcec.PublicKey

	// chanPoint is the final channel point for the to be created channel.
	chanPoint wire.OutPoint

	// initiator indicates if we're the initiator or the channel or not.
	initiator bool
}

// NewCannedAssembler creates a new CannedAssembler from the material required
// to construct a funding output and channel point.
func NewCannedAssembler(chanPoint wire.OutPoint, fundingAmt ltcutil.Amount,
	localKey *keychain.KeyDescriptor,
	remoteKey *btcec.PublicKey, initiator bool) *CannedAssembler {

	return &CannedAssembler{
		initiator:  initiator,
		localKey:   localKey,
		remoteKey:  remoteKey,
		fundingAmt: fundingAmt,
		chanPoint:  chanPoint,
	}
}

// ProvisionChannel creates a new ShimIntent given the passed funding Request.
// The returned intent is immediately able to provide the channel point and
// funding output as they've already been created outside lnd.
//
// NOTE: This method satisfies the chanfunding.Assembler interface.
func (c *CannedAssembler) ProvisionChannel(req *Request) (Intent, error) {
	// We'll exit out if this field is set as the funding transaction has
	// already been assembled, so we don't influence coin selection..
	if req.SubtractFees {
		return nil, fmt.Errorf("SubtractFees ignored, funding " +
			"transaction is frozen")
	}

	intent := &ShimIntent{
		localKey:  c.localKey,
		remoteKey: c.remoteKey,
		chanPoint: &c.chanPoint,
	}

	if c.initiator {
		intent.localFundingAmt = c.fundingAmt
	} else {
		intent.remoteFundingAmt = c.fundingAmt
	}

	// A simple sanity check to ensure the provisioned request matches the
	// re-made shim intent.
	if req.LocalAmt+req.RemoteAmt != c.fundingAmt {
		return nil, fmt.Errorf("intent doesn't match canned "+
			"assembler: local_amt=%v, remote_amt=%v, funding_amt=%v",
			req.LocalAmt, req.RemoteAmt, c.fundingAmt)
	}

	return intent, nil
}

// A compile-time assertion to ensure CannedAssembler meets the Assembler
// interface.
var _ Assembler = (*CannedAssembler)(nil)
