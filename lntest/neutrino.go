// +build neutrino

package lntest

import (
	"fmt"

	"github.com/ltcsuite/ltcd/chaincfg"
)

// NeutrinoBackendConfig is an implementation of the BackendConfig interface
// backed by a neutrino node.
type NeutrinoBackendConfig struct {
	minerAddr string
}

// A compile time assertion to ensure NeutrinoBackendConfig meets the
// BackendConfig interface.
var _ BackendConfig = (*NeutrinoBackendConfig)(nil)

// GenArgs returns the arguments needed to be passed to LND at startup for
// using this node as a chain backend.
func (b NeutrinoBackendConfig) GenArgs() []string {
	var args []string
	args = append(args, "--bitcoin.node=neutrino")
	args = append(args, "--neutrino.connect="+b.minerAddr)
	return args
}

// ConnectMiner is called to establish a connection to the test miner.
func (b NeutrinoBackendConfig) ConnectMiner() error {
	return fmt.Errorf("unimplemented")
}

// DisconnectMiner is called to disconnect the miner.
func (b NeutrinoBackendConfig) DisconnectMiner() error {
	return fmt.Errorf("unimplemented")
}

// Name returns the name of the backend type.
func (b NeutrinoBackendConfig) Name() string {
	return "neutrino"
}

// NewBackend starts and returns a NeutrinoBackendConfig for the node.
func NewBackend(miner string, _ *chaincfg.Params) (
	*NeutrinoBackendConfig, func(), error) {

	bd := &NeutrinoBackendConfig{
		minerAddr: miner,
	}

	cleanUp := func() {}
	return bd, cleanUp, nil
}
