package member

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// memberIdentityDomain separates member issuance from every other identity
// derivation, so an axis entry and a member of that axis can never collide.
const memberIdentityDomain = "wippy.analysis/schema/axis/member/identity/v1"

// IssueID is the one member identity assigner.
//
// A member is nested in exactly one axis entry and a member key is unique
// within that entry's catalog whatever kind the member is, so the axis entry's
// own issued identity extended by the member key names the member exactly.
// Relations, projections, reducers and carry transforms are therefore issued
// by one assigner rather than four, and a consumer that needs a member
// identity asks here instead of deriving one of its own.
//
// It fails closed: an unavailable axis reference or member key issues the zero
// identity, which reports itself unavailable rather than passing as a real
// one.
func IssueID(axis schema.EntryReference, key schema.Key) schema.EntryID {
	if axis.Surface != schema.SurfaceKindAxis || !axis.Key.Available() || !key.Available() {
		return schema.EntryID{}
	}
	entry := schema.NewEntryID(axis.Surface, axis.Key)
	if !entry.Available() {
		return schema.EntryID{}
	}
	issued, ok := identity.DeriveContentID(memberIdentityDomain, entryBytes(entry), []byte(key))
	if !ok {
		return schema.EntryID{}
	}
	return schema.EntryID(issued)
}

// ID returns the identity the owning axis issued for this relation member.
func (reference RelationRef) ID() schema.EntryID {
	return IssueID(reference.Axis, reference.Member)
}

// ID returns the identity the owning axis issued for this projection member.
func (reference ProjectionRef) ID() schema.EntryID {
	return IssueID(reference.Axis, reference.Member)
}

// ID returns the identity the owning axis issued for this reducer member.
func (reference ReducerRef) ID() schema.EntryID {
	return IssueID(reference.Axis, reference.Member)
}

// ID returns the identity the owning axis issued for this carry transform.
func (reference CarryTransformRef) ID() schema.EntryID {
	return IssueID(reference.Axis, reference.Member)
}

func entryBytes(entry schema.EntryID) []byte {
	value := identity.ContentID(entry)
	return append([]byte(nil), value[:]...)
}
