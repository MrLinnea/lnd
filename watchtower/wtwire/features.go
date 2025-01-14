package wtwire

import "github.com/ltcsuite/lnd/lnwire"

// FeatureNames holds a mapping from each feature bit understood by this
// implementation to its common name.
var FeatureNames = map[lnwire.FeatureBit]string{
	AltruistSessionsRequired: "altruist-sessions",
	AltruistSessionsOptional: "altruist-sessions",
	AnchorCommitRequired:     "anchor-commit",
	AnchorCommitOptional:     "anchor-commit",
}

const (
	// AltruistSessionsRequired specifies that the advertising node requires
	// the remote party to understand the protocol for creating and updating
	// watchtower sessions.
	AltruistSessionsRequired lnwire.FeatureBit = 0

	// AltruistSessionsOptional specifies that the advertising node can
	// support a remote party who understand the protocol for creating and
	// updating watchtower sessions.
	AltruistSessionsOptional lnwire.FeatureBit = 1

	// AnchorCommitRequired specifies that the advertising tower requires
	// the remote party to negotiate sessions for protecting anchor
	// channels.
	AnchorCommitRequired lnwire.FeatureBit = 2

	// AnchorCommitOptional specifies that the advertising tower allows the
	// remote party to negotiate sessions for protecting anchor channels.
	AnchorCommitOptional lnwire.FeatureBit = 3
)
