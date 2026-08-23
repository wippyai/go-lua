package placement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// EvidenceState is the four-valued state used by Placement-owned proof
// columns. The four states are three distinct answers plus the absence of an
// answer, and the distinction between them is load-bearing:
//
//   - Absent: no producer wrote this column. Nothing was decided because
//     nothing was asked. A consumer that needs the column refuses and the
//     engine refires the query when the producing row arrives.
//   - Unknown: a producer read its complete input and authenticated that no
//     polarity survives. This is a settled semantic verdict, not a pending
//     one.
//   - Refuted / Proven: a producer established the property's negation or
//     the property.
//
// Absence is the state's zero value and its join identity, mirroring the
// suspension producer's own private vocabulary, where the sparse Factor
// default is likewise the join identity. Collapsing Absent into Unknown at any
// boundary publishes a verdict no producer reached.
//
// This state is part of Placement's query/result contract; it replaces the
// older hand-built allocation-license projection.
type EvidenceState uint8

const (
	EvidenceAbsent EvidenceState = iota
	EvidenceUnknown
	EvidenceRefuted
	EvidenceProven
)

// Valid reports whether s is one of the wire-safe proof states.
func (s EvidenceState) Valid() bool { return s <= EvidenceProven }

// Absent reports whether no producer published this column.
func (s EvidenceState) Absent() bool { return s == EvidenceAbsent }

// Known reports whether a producer supplied either a positive or a negative
// proof.
func (s EvidenceState) Known() bool { return s == EvidenceRefuted || s == EvidenceProven }

// Proven reports whether the producer established the property.
func (s EvidenceState) Proven() bool { return s == EvidenceProven }

// Refuted reports whether the producer established the property's negation.
func (s EvidenceState) Refuted() bool { return s == EvidenceRefuted }

// JoinChecked combines two authenticated proof states. Absence is the
// identity: an unwritten column contributes nothing and can neither erase nor
// weaken the state it joins with. An authenticated Unknown is a completed
// semantic verdict, not a weaker spelling of Proven or Refuted. Consequently
// two written states compose only when they are exactly equal; every distinct
// pair is duplicate/conflicting authority and is refused.
// A caller that has independently authenticated a semantic disagreement may
// deliberately publish EvidenceUnknown, but that decision belongs to that
// producer rather than to this scalar composition primitive.
func (s EvidenceState) JoinChecked(other EvidenceState) (EvidenceState, bool) {
	if !s.Valid() || !other.Valid() {
		return invalidEvidenceState, false
	}
	if s == EvidenceAbsent {
		return other, true
	}
	if other == EvidenceAbsent {
		return s, true
	}
	if s == other {
		return s, true
	}
	return invalidEvidenceState, false
}

const invalidEvidenceState EvidenceState = ^EvidenceState(0)

// InvalidEvidenceState is the explicit refusal state for a producer whose own
// private vocabulary has no public projection for a value. It is deliberately
// outside the admissible states, so a boundary that forgets to check a
// projection's provenance still refuses on Valid rather than publishing a
// substituted verdict.
func InvalidEvidenceState() EvidenceState { return invalidEvidenceState }

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
// columns carry EvidenceAbsent when their producer did not publish a row, so
// an unwritten column is never read as a producer-authenticated verdict.
//
// OwnerIdentity is the identity of this owner-issued Heap root. It is not a
// containment-parent identity. Static containment depth, when available, is
// joined by the heterogeneous Placement+Heap query using Heap's
// owner-authenticated summary vector.
type AllocationEvidence struct {
	// Class is copied from the Placement factor when a row is present. It is
	// kept here so a consumer can inspect one complete evidence row without
	// separately retaining the iterator's class accessor.
	Class    Placement
	HasClass bool
	// RetainEscape is the path-sensitive provenance carried by the canonical
	// Placement factor at the sampled point. It is not inferred from Class.
	RetainEscape         EvidenceState
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
	if e.HasClass == e.RetainEscape.Absent() {
		// Class and retain provenance are the two components of one canonical
		// factor fact; neither may be published without the other.
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
		// Do not let a stale scalar survive without its presence bit. Strict
		// composition refuses conflicts; it never clears a conflicting field.
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
	if !e.RetainEscape.Valid() || !e.FrameLocal.Valid() || !e.DiesBeforeSuspension.Valid() || !e.DeepFrozen.Valid() {
		return false
	}
	if !e.HasDepth && e.Depth != 0 {
		return false
	}
	return true
}

// ComposeAllocationEvidence combines the canonical Heap row with one
// producer-owned refinement. It is deliberately strict: absent optional
// fields do not erase an existing fact, equal present scalars may be repeated,
// but conflicting owner identity, kind, class, depth, or proof polarity
// refuses the row. This prevents a malformed/foreign producer from being
// "conservatively" erased into a valid empty record and later published.
//
// Semantic disagreement is not inferred here. A producer that has a real
// authenticated alternate-world proof may set its own proof field to
// EvidenceUnknown before calling this function; the generic composition
// layer has no authority to manufacture that widening.
func ComposeAllocationEvidence(base, producer AllocationEvidence) (AllocationEvidence, bool) {
	if !base.Valid() || !producer.Valid() {
		return invalidAllocationEvidence(), false
	}
	result := base
	if producer.HasKind {
		if result.HasKind && result.Kind != producer.Kind {
			return invalidAllocationEvidence(), false
		}
		result.HasKind = true
		result.Kind = producer.Kind
	}
	if producer.HasClass {
		if result.HasClass && result.Class != producer.Class {
			return invalidAllocationEvidence(), false
		}
		result.HasClass = true
		result.Class = producer.Class
	}
	var ok bool
	if result.RetainEscape, ok = result.RetainEscape.JoinChecked(producer.RetainEscape); !ok {
		return invalidAllocationEvidence(), false
	}
	if producer.HasOwnerIdentity {
		if result.HasOwnerIdentity && result.OwnerIdentity != producer.OwnerIdentity {
			return invalidAllocationEvidence(), false
		}
		result.HasOwnerIdentity = true
		result.OwnerIdentity = producer.OwnerIdentity
	}
	if producer.HasDepth {
		if result.HasDepth && result.Depth != producer.Depth {
			return invalidAllocationEvidence(), false
		}
		result.HasDepth = true
		result.Depth = producer.Depth
	}
	if result.FrameLocal, ok = result.FrameLocal.JoinChecked(producer.FrameLocal); !ok {
		return invalidAllocationEvidence(), false
	}
	if result.DiesBeforeSuspension, ok = result.DiesBeforeSuspension.JoinChecked(producer.DiesBeforeSuspension); !ok {
		return invalidAllocationEvidence(), false
	}
	if result.DeepFrozen, ok = result.DeepFrozen.JoinChecked(producer.DeepFrozen); !ok {
		return invalidAllocationEvidence(), false
	}
	if !result.Valid() {
		return invalidAllocationEvidence(), false
	}
	return result, true
}

// invalidAllocationEvidence is the explicit refusal value for checked
// evidence composition. The zero AllocationEvidence is valid all-absent
// evidence, so it cannot represent failure without relying on callers to
// inspect a boolean at every boundary.
func invalidAllocationEvidence() AllocationEvidence {
	return AllocationEvidence{Class: invalidPlacementResult, HasClass: true}
}

// allocationEvidenceForKey derives the complete evidence currently justified
// by Placement's own factor and Heap coordinate. It intentionally leaves all
// columns without an authenticated producer absent.
func allocationEvidenceForKey(schema Schema, key heapdomain.Key, fact Fact, present bool) (AllocationEvidence, bool) {
	if !schema.Valid() || !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return invalidAllocationEvidence(), false
	}
	if !fact.Valid() || present && (fact.Class == Bottom || fact.RetainEscape == EvidenceAbsent) {
		return invalidAllocationEvidence(), false
	}
	id, idOK := key.ContentID()
	if !idOK || !id.Available() {
		return invalidAllocationEvidence(), false
	}
	evidence := AllocationEvidence{OwnerIdentity: id, HasOwnerIdentity: true}
	if present {
		evidence.Class, evidence.HasClass = fact.Class, true
		evidence.RetainEscape = fact.RetainEscape
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
		switch fact.Class {
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
