package placement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// EvidenceState is the three-valued state used by Placement-owned proof
// columns. Unknown is different from Refuted: an absent producer must not be
// turned into a negative fact merely because a consumer is conservative.
//
// This state is part of Placement's query/result contract; it replaces the
// older hand-built allocation-license projection.
type EvidenceState uint8

const (
	EvidenceUnknown EvidenceState = iota
	EvidenceRefuted
	EvidenceProven
)

// Valid reports whether s is one of the wire-safe proof states.
func (s EvidenceState) Valid() bool {
	return s == EvidenceUnknown || s == EvidenceRefuted || s == EvidenceProven
}

// Known reports whether a producer supplied either a positive or a negative
// proof.
func (s EvidenceState) Known() bool { return s == EvidenceRefuted || s == EvidenceProven }

// Proven reports whether the producer established the property.
func (s EvidenceState) Proven() bool { return s == EvidenceProven }

// Refuted reports whether the producer established the property's negation.
func (s EvidenceState) Refuted() bool { return s == EvidenceRefuted }

// Join is the conservative consensus join for alternate evidence sources.
// Proven or Refuted survives only when every explicit source agrees; mixed
// polarity and any Unknown source remain Unknown.
func (s EvidenceState) Join(other EvidenceState) EvidenceState {
	if !s.Valid() || !other.Valid() {
		return EvidenceUnknown
	}
	if s == EvidenceUnknown || other == EvidenceUnknown {
		return EvidenceUnknown
	}
	if s == other {
		return s
	}
	return EvidenceUnknown
}

// AllocationKind is the allocation-origin vocabulary backed by Heap. Program
// roots retain their concrete Lua allocation role; Target-authored fresh roots
// retain the generic manifest allocation origin proven by FreshResultID.
// Unknown is represented by absence of HasKind on AllocationEvidence.
type AllocationKind uint8

const (
	AllocationKindUnknown AllocationKind = iota
	AllocationKindTable
	AllocationKindClosure
	AllocationKindManifest
)

// Valid reports whether k is a known Heap-backed kind. Unknown is a valid
// value for an unavailable kind, while HasKind must be false for it.
func (k AllocationKind) Valid() bool { return k <= AllocationKindManifest }

// String returns the canonical diagnostic spelling.
func (k AllocationKind) String() string {
	switch k {
	case AllocationKindTable:
		return "lua.table"
	case AllocationKindClosure:
		return "lua.closure"
	case AllocationKindManifest:
		return "manifest.allocation"
	case AllocationKindUnknown:
		return "unknown"
	default:
		return "placement-kind(invalid)"
	}
}

// AllocationEvidence is the Placement-owned evidence plane for one Heap
// allocation root. Optional scalar columns carry explicit Has* bits; proof
// columns carry EvidenceUnknown when their producer did not publish a row.
//
// OwnerIdentity is the identity of this owner-issued Heap root. It is not a
// containment-parent identity. Static containment depth, when available, is
// joined by the heterogeneous Placement+Heap query using Heap's
// owner-authenticated summary vector.
type AllocationEvidence struct {
	// Class is copied from the Placement factor when a row is present. It is
	// kept here so a consumer can inspect one complete evidence row without
	// separately retaining the iterator's class accessor.
	Class                Placement
	HasClass             bool
	Kind                 AllocationKind
	HasKind              bool
	OwnerIdentity        identity.ContentID
	HasOwnerIdentity     bool
	Depth                uint32
	HasDepth             bool
	FrameLocal           EvidenceState
	DiesBeforeSuspension EvidenceState
	// DeepFrozen is the transitive publication-safety proof derived from the
	// complete Heap summary. It is deliberately a proof column rather than a
	// Placement class: a shared allocation may still be deeply frozen, while
	// an owned allocation may contain a mutable descendant.
	DeepFrozen EvidenceState
}

// Valid reports whether all present fields can be admitted to Placement's
// evidence codec. Unknown optional fields are valid only when their presence
// bit is clear.
func (e AllocationEvidence) Valid() bool {
	if e.HasClass && !validAnalysisPlacement(e.Class) {
		return false
	}
	if !e.HasClass && e.Class != Bottom {
		return false
	}
	if !e.Kind.Valid() {
		return false
	}
	if e.HasKind {
		// Unknown is represented by the absence of the optional kind column;
		// a present unknown kind would make the wire plane ambiguous.
		if e.Kind == AllocationKindUnknown {
			return false
		}
	} else if e.Kind != AllocationKindUnknown {
		// Do not let a stale scalar survive after its presence bit was cleared.
		// This is especially important after a conservative Merge conflict:
		// the absent value must be the exact zero/unknown sentinel.
		return false
	}
	if e.HasOwnerIdentity {
		if !e.OwnerIdentity.Available() {
			return false
		}
	} else if e.OwnerIdentity.Available() {
		// OwnerIdentity is an owner-issued optional scalar. A value without its
		// presence bit is a detached identity that no consumer may trust.
		return false
	}
	if !e.FrameLocal.Valid() || !e.DiesBeforeSuspension.Valid() || !e.DeepFrozen.Valid() {
		return false
	}
	if !e.HasDepth && e.Depth != 0 {
		return false
	}
	return true
}

// Merge overlays independently supplied optional evidence while preserving
// the conservative unknown state on disagreement. A missing optional field in
// the right operand does not erase a fact derived from Heap; this is what lets
// the canonical Heap projection coexist with a future neutral proof producer.
func (e AllocationEvidence) Merge(other AllocationEvidence) AllocationEvidence {
	if !e.Valid() || !other.Valid() {
		return AllocationEvidence{}
	}
	if other.HasKind {
		if e.HasKind && e.Kind != other.Kind {
			e.HasKind = false
			e.Kind = AllocationKindUnknown
		} else {
			e.HasKind = true
			e.Kind = other.Kind
		}
	}
	if other.HasClass {
		if e.HasClass && e.Class != other.Class {
			e.HasClass = false
			e.Class = Bottom
		} else {
			e.HasClass = true
			e.Class = other.Class
		}
	}
	if other.HasOwnerIdentity {
		if e.HasOwnerIdentity && e.OwnerIdentity != other.OwnerIdentity {
			e.HasOwnerIdentity = false
			e.OwnerIdentity = identity.ContentID{}
		} else {
			e.HasOwnerIdentity = true
			e.OwnerIdentity = other.OwnerIdentity
		}
	}
	if other.HasDepth {
		if e.HasDepth && e.Depth != other.Depth {
			e.HasDepth = false
			e.Depth = 0
		} else {
			e.HasDepth = true
			e.Depth = other.Depth
		}
	}
	e.FrameLocal = mergeEvidenceState(e.FrameLocal, other.FrameLocal)
	e.DiesBeforeSuspension = mergeEvidenceState(e.DiesBeforeSuspension, other.DiesBeforeSuspension)
	e.DeepFrozen = mergeEvidenceState(e.DeepFrozen, other.DeepFrozen)
	return e
}

func mergeEvidenceState(left, right EvidenceState) EvidenceState {
	if !left.Valid() || !right.Valid() {
		return EvidenceUnknown
	}
	if right == EvidenceUnknown {
		return left
	}
	if left == EvidenceUnknown || left == right {
		return right
	}
	// Conflicting producers have not established a stable property. Preserve
	// a negative fact only when both producers agree on the negation.
	return EvidenceUnknown
}

// allocationEvidenceForKey derives the complete evidence currently justified
// by Placement's own factor and Heap coordinate. It intentionally leaves all
// dimensions without a producer unknown.
func allocationEvidenceForKey(schema Schema, key heapdomain.Key, class Placement, present bool) (AllocationEvidence, bool) {
	if !schema.Valid() || !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return AllocationEvidence{}, false
	}
	id, idOK := key.ContentID()
	if !idOK || !id.Available() {
		return AllocationEvidence{}, false
	}
	evidence := AllocationEvidence{OwnerIdentity: id, HasOwnerIdentity: true}
	if present && validAnalysisPlacement(class) {
		evidence.Class, evidence.HasClass = class, true
	}
	if _, _, _, kind, _, originOK := schema.Heap().AllocationOriginForKey(key); originOK {
		switch kind {
		case heapdomain.AllocationTable:
			evidence.Kind, evidence.HasKind = AllocationKindTable, true
		case heapdomain.AllocationClosure:
			evidence.Kind, evidence.HasKind = AllocationKindClosure, true
		}
	} else if _, _, _, freshOK := key.FreshResultID(); freshOK {
		evidence.Kind, evidence.HasKind = AllocationKindManifest, true
	}
	if present {
		switch class {
		case Stack:
			evidence.FrameLocal = EvidenceProven
		case OwnedHeap, SharedHeap, Unknown:
			// A class at or above OwnedHeap is evidence that the allocation
			// cannot be frame-local. This does not assert any storage-lifetime
			// or suspension route; those remain separate columns.
			evidence.FrameLocal = EvidenceRefuted
		}
	}
	return evidence, true
}
