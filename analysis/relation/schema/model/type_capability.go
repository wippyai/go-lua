package model

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// TypeCapabilityKind is the sealed authority carried by one owner-issued
// TypeID.  A codec admits an exact owner value into a frame; only Ascending
// also admits the value to monotone relation state.  The two meanings are
// deliberately separate: presence, Go representation, and an operation's
// output contract never manufacture an ascent authority.
type TypeCapabilityKind uint8

const (
	// InvalidTypeCapability is the unavailable zero value.
	InvalidTypeCapability TypeCapabilityKind = iota
	// DecodeOnly admits exact owner-issued values and authenticated opaque
	// tokens, but has no Join/LessOrEqual/Widen authority.
	DecodeOnly
	// Equatable admits owner-defined semantic equality for key operations, but
	// does not admit lattice ascent.
	Equatable
	// Ascending admits exact values and one owner-issued lattice authority.
	Ascending
)

// String returns the stable sealed capability vocabulary.
func (kind TypeCapabilityKind) String() string {
	switch kind {
	case DecodeOnly:
		return "DecodeOnly"
	case Equatable:
		return "Equatable"
	case Ascending:
		return "Ascending"
	default:
		return "InvalidTypeCapability"
	}
}

// TypeCapability is the schema-owned, digestable policy for one nominal
// TypeID. It is ordered by authority: DecodeOnly < Equatable < Ascending.
// Equatable is deliberately not a lattice; key operators need semantic
// equality without Join/Widen/LessOrEqual. The schema compiler carries the
// owner-issued identity and this policy together, then seals the resulting
// content into the execution-schema digest. Consumers compare the capability;
// they never infer it from Presence or from a concrete Go type.
type TypeCapability struct {
	typeID TypeID
	kind   TypeCapabilityKind
}

// NewTypeCapability seals one explicit policy for typeID.
func NewTypeCapability(typeID TypeID, kind TypeCapabilityKind) (TypeCapability, bool) {
	if !typeID.Available() || (kind != DecodeOnly && kind != Equatable && kind != Ascending) {
		return TypeCapability{}, false
	}
	return TypeCapability{typeID: typeID, kind: kind}, true
}

// NewDecodeOnlyCapability seals the exact-codec-only policy.
func NewDecodeOnlyCapability(typeID TypeID) (TypeCapability, bool) {
	return NewTypeCapability(typeID, DecodeOnly)
}

// NewEquatableCapability seals the key-equality-only policy.
func NewEquatableCapability(typeID TypeID) (TypeCapability, bool) {
	return NewTypeCapability(typeID, Equatable)
}

// NewAscendingCapability seals the owner-lattice policy.
func NewAscendingCapability(typeID TypeID) (TypeCapability, bool) {
	return NewTypeCapability(typeID, Ascending)
}

// Available reports whether the policy is complete.
func (capability TypeCapability) Available() bool {
	return capability.typeID.Available() && capability.kind != InvalidTypeCapability
}

// Type returns the exact owner-issued TypeID governed by this capability.
func (capability TypeCapability) Type() TypeID { return capability.typeID }

// Kind returns the sealed policy kind.
func (capability TypeCapability) Kind() TypeCapabilityKind { return capability.kind }

// DecodeOnly reports whether the type is transportable but has no semantic
// equality or lattice authority.
func (capability TypeCapability) DecodeOnly() bool { return capability.kind == DecodeOnly }

// Equatable reports whether the type can be used by semantic key operators.
// Ascending includes equality because its owner order is the canonical
// equality witness; it still remains a distinct lattice authority.
func (capability TypeCapability) Equatable() bool {
	return capability.kind == Equatable || capability.kind == Ascending
}

// Ascending reports whether the type may enter lattice ascent.
func (capability TypeCapability) Ascending() bool { return capability.kind == Ascending }

// Digest returns the stable content identity of this policy.  Including the
// TypeID and capability kind makes a decode-only/ascending change a schema
// identity change even when every row, operation, and Presence contract is
// unchanged.
func (capability TypeCapability) Digest() identity.ContentID {
	if !capability.Available() {
		return identity.ContentID{}
	}
	typeOwner := capability.typeID.Owner().Content()
	typeContent := capability.typeID.Content()
	value, ok := identity.DeriveContentID(
		"analysis/relation/schema/model/type-capability/v1",
		typeOwner[:], typeContent[:], []byte{byte(capability.kind)},
	)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

// Equal compares both the nominal type and the sealed policy.
func (capability TypeCapability) Equal(other TypeCapability) bool {
	return capability == other
}
