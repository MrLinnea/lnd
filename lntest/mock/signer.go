package mock

import (
	"fmt"

	"github.com/ltcsuite/lnd/input"
	"github.com/ltcsuite/lnd/keychain"
	"github.com/ltcsuite/ltcd/btcec/v2"
	"github.com/ltcsuite/ltcd/btcec/v2/ecdsa"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
)

var (
	idKeyLoc = keychain.KeyLocator{Family: keychain.KeyFamilyNodeKey}
)

// DummySignature is a dummy Signature implementation.
type DummySignature struct{}

// Serialize returns an empty byte slice.
func (d *DummySignature) Serialize() []byte {
	return []byte{}
}

// Verify always returns true.
func (d *DummySignature) Verify(_ []byte, _ *btcec.PublicKey) bool {
	return true
}

// DummySigner is an implementation of the Signer interface that returns
// dummy values when called.
type DummySigner struct{}

// SignOutputRaw returns a dummy signature.
func (d *DummySigner) SignOutputRaw(tx *wire.MsgTx,
	signDesc *input.SignDescriptor) (input.Signature, error) {

	return &DummySignature{}, nil
}

// ComputeInputScript returns nil for both values.
func (d *DummySigner) ComputeInputScript(tx *wire.MsgTx,
	signDesc *input.SignDescriptor) (*input.Script, error) {

	return &input.Script{}, nil
}

// SingleSigner is an implementation of the Signer interface that signs
// everything with a single private key.
type SingleSigner struct {
	Privkey *btcec.PrivateKey
	KeyLoc  keychain.KeyLocator
}

// SignOutputRaw generates a signature for the passed transaction using the
// stored private key.
func (s *SingleSigner) SignOutputRaw(tx *wire.MsgTx,
	signDesc *input.SignDescriptor) (input.Signature, error) {

	amt := signDesc.Output.Value
	witnessScript := signDesc.WitnessScript
	privKey := s.Privkey

	if !privKey.PubKey().IsEqual(signDesc.KeyDesc.PubKey) {
		return nil, fmt.Errorf("incorrect key passed")
	}

	switch {
	case signDesc.SingleTweak != nil:
		privKey = input.TweakPrivKey(privKey,
			signDesc.SingleTweak)
	case signDesc.DoubleTweak != nil:
		privKey = input.DeriveRevocationPrivKey(privKey,
			signDesc.DoubleTweak)
	}

	sig, err := txscript.RawTxInWitnessSignature(tx, signDesc.SigHashes,
		signDesc.InputIndex, amt, witnessScript, signDesc.HashType,
		privKey)
	if err != nil {
		return nil, err
	}

	return ecdsa.ParseDERSignature(sig[:len(sig)-1])
}

// ComputeInputScript computes an input script with the stored private key
// given a transaction and a SignDescriptor.
func (s *SingleSigner) ComputeInputScript(tx *wire.MsgTx,
	signDesc *input.SignDescriptor) (*input.Script, error) {

	privKey := s.Privkey

	switch {
	case signDesc.SingleTweak != nil:
		privKey = input.TweakPrivKey(privKey,
			signDesc.SingleTweak)
	case signDesc.DoubleTweak != nil:
		privKey = input.DeriveRevocationPrivKey(privKey,
			signDesc.DoubleTweak)
	}

	witnessScript, err := txscript.WitnessSignature(tx, signDesc.SigHashes,
		signDesc.InputIndex, signDesc.Output.Value, signDesc.Output.PkScript,
		signDesc.HashType, privKey, true)
	if err != nil {
		return nil, err
	}

	return &input.Script{
		Witness: witnessScript,
	}, nil
}

// SignMessage takes a public key and a message and only signs the message
// with the stored private key if the public key matches the private key.
func (s *SingleSigner) SignMessage(keyLoc keychain.KeyLocator,
	msg []byte, doubleHash bool) (*ecdsa.Signature, error) {

	mockKeyLoc := s.KeyLoc
	if s.KeyLoc.IsEmpty() {
		mockKeyLoc = idKeyLoc
	}

	if keyLoc != mockKeyLoc {
		return nil, fmt.Errorf("unknown public key")
	}

	var digest []byte
	if doubleHash {
		digest = chainhash.DoubleHashB(msg)
	} else {
		digest = chainhash.HashB(msg)
	}
	return ecdsa.Sign(s.Privkey, digest), nil
}
