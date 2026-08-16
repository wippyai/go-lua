package link

import "github.com/wippyai/go-lua/analysis/identity"

// OwnerCapability is the exact owner witness issued by one sealed Link.
//
// The witness is deliberately a copyable, opaque capability: its identity is
// the private state pointer, rather than ContentID (which is replayable and
// therefore cannot distinguish independently sealed equal-content Links).
// The state contains no Link, Project, Program, or Flow pointer, so retaining
// a capability cannot retain the sealed source graph.
type OwnerCapability struct {
	state *ownerState
}

type ownerState struct {
	id identity.ContentID
}

// Available reports whether this is an issued capability.
func (capability OwnerCapability) Available() bool {
	return capability.state != nil && capability.state.id.Available()
}

// ContentID returns the detached Link content identity carried alongside the
// exact owner witness. It is an identity hint only; callers must use Matches
// to authenticate ownership.
func (capability OwnerCapability) ContentID() identity.ContentID {
	if capability.state == nil {
		return identity.ContentID{}
	}
	return capability.state.id
}

// Matches proves exact owner identity. Copies of the same capability match;
// equal-content capabilities issued by another Link do not.
func (capability OwnerCapability) Matches(other OwnerCapability) bool {
	return capability.state != nil && capability.state == other.state
}

// Equal is an explicit spelling of Matches for generic owner-fence helpers.
func (capability OwnerCapability) Equal(other OwnerCapability) bool {
	return capability.Matches(other)
}

// OwnerCapability issues Link's exact detached owner witness.
func (l *Link) OwnerCapability() OwnerCapability {
	if l == nil {
		return OwnerCapability{}
	}
	return OwnerCapability{state: l.owner}
}
