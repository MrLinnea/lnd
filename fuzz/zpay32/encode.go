//go:build gofuzz
// +build gofuzz

package zpay32fuzz

import (
	"encoding/hex"
	"fmt"

	"github.com/ltcsuite/lnd/zpay32"
	"github.com/ltcsuite/ltcd/btcec/v2"
	"github.com/ltcsuite/ltcd/btcec/v2/ecdsa"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
)

// Fuzz_encode is used by go-fuzz.
func Fuzz_encode(data []byte) int {
	inv, err := zpay32.Decode(string(data), &chaincfg.TestNet4Params)
	if err != nil {
		return 1
	}

	// Call these functions as a sanity check to make sure the invoice
	// is well-formed.
	_ = inv.MinFinalCLTVExpiry()
	_ = inv.Expiry()

	// Initialize the static key we will be using for this fuzz test.
	testPrivKeyBytes, _ := hex.DecodeString("e126f68f7eafcc8b74f54d269fe206be715000f94dac067d1c04a8ca3b2db734")
	testPrivKey, _ := btcec.PrivKeyFromBytes(testPrivKeyBytes)

	// Then, initialize the testMessageSigner so we can encode out
	// invoices with this private key.
	testMessageSigner := zpay32.MessageSigner{
		SignCompact: func(msg []byte) ([]byte, error) {
			hash := chainhash.HashB(msg)
			sig, err := ecdsa.SignCompact(testPrivKey, hash, true)
			if err != nil {
				return nil, fmt.Errorf("can't sign the "+
					"message: %v", err)
			}
			return sig, nil
		},
	}
	_, err = inv.Encode(testMessageSigner)
	if err != nil {
		return 1
	}

	return 1
}
