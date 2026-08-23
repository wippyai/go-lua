package placement

import (
	"github.com/wippyai/go-lua/analysis/lattice"
	internal "github.com/wippyai/go-lua/internal/hash"
)

// Fact is the one canonical Placement factor value for an allocation at one
// Program point. Class answers where the allocation must live. RetainEscape
// answers whether a retaining boundary has already been crossed on every path
// reaching that same point.
//
// Keeping both components in the factor is load-bearing: a scalar Placement
// class loses the provenance when Store and Send both reach SharedHeap, while
// a detached companion factor would create a second authority and could drift
// from the displacement that established the class.
type Fact struct {
	Class        Placement
	RetainEscape EvidenceState
}

// String returns the stable diagnostic spelling of the placement fact.
func (fact Fact) String() string {
	return "class=" + fact.Class.String() + "/retain=" + evidenceStateString(fact.RetainEscape)
}

func evidenceStateString(state EvidenceState) string {
	switch state {
	case EvidenceAbsent:
		return "absent"
	case EvidenceRefuted:
		return "refuted"
	case EvidenceUnknown:
		return "unknown"
	case EvidenceProven:
		return "proven"
	default:
		return "invalid"
	}
}

// BottomFact is the unreachable product value used by the lattice. It is not
// an admitted allocation cell.
func BottomFact() Fact { return Fact{Class: Bottom, RetainEscape: EvidenceAbsent} }

// DefaultFact is the owner-issued value for every allocation coordinate. The
// allocation exists at Stack and no retaining boundary precedes its birth.
func DefaultFact() Fact { return Fact{Class: Stack, RetainEscape: EvidenceRefuted} }

// UnknownFact is the authenticated top of both Placement components.
func UnknownFact() Fact { return Fact{Class: Unknown, RetainEscape: EvidenceUnknown} }

// Valid admits the complete product lattice, including component-bottom
// values produced by Meet. Factor-cell authentication applies the stricter
// reachable-cell contract separately.
func (fact Fact) Valid() bool {
	return validAnalysisPlacement(fact.Class) && fact.RetainEscape.Valid()
}

// Hash fingerprints the complete canonical factor value.
func (fact Fact) Hash() uint64 {
	if !fact.Valid() {
		return 0
	}
	return internal.MixHash(fact.Class.Hash(), uint64(fact.RetainEscape)+1)
}

// FactLattice is the product of the Placement chain and the four-state
// control-flow proof lattice. Refuted and Proven are incomparable; joining
// paths carrying the two answers yields Unknown rather than inventing either
// polarity.
func FactLattice() lattice.Lattice[Fact] {
	return lattice.Lattice[Fact]{
		Bottom:   BottomFact,
		Top:      UnknownFact,
		Equal:    EqualFact,
		LessOrEq: LessOrEqFact,
		Join:     JoinFact,
		Meet:     MeetFact,
		Widen:    JoinFact,
	}
}

func EqualFact(left, right Fact) bool {
	return left.Valid() && right.Valid() && left == right
}

func LessOrEqFact(left, right Fact) bool {
	return left.Valid() && right.Valid() &&
		LessOrEq(left.Class, right.Class) && retainEvidenceLessOrEq(left.RetainEscape, right.RetainEscape)
}

func JoinFactChecked(left, right Fact) (Fact, bool) {
	if !left.Valid() || !right.Valid() {
		return invalidFact(), false
	}
	class, classOK := JoinChecked(left.Class, right.Class)
	retained, retainedOK := joinRetainEvidence(left.RetainEscape, right.RetainEscape)
	if !classOK || !retainedOK {
		return invalidFact(), false
	}
	joined := Fact{Class: class, RetainEscape: retained}
	return joined, true
}

func JoinFact(left, right Fact) Fact {
	joined, ok := JoinFactChecked(left, right)
	if !ok {
		return invalidFact()
	}
	return joined
}

func MeetFactChecked(left, right Fact) (Fact, bool) {
	if !left.Valid() || !right.Valid() {
		return invalidFact(), false
	}
	class, classOK := MeetChecked(left.Class, right.Class)
	retained, retainedOK := meetRetainEvidence(left.RetainEscape, right.RetainEscape)
	if !classOK || !retainedOK {
		return invalidFact(), false
	}
	return Fact{Class: class, RetainEscape: retained}, true
}

func MeetFact(left, right Fact) Fact {
	met, ok := MeetFactChecked(left, right)
	if !ok {
		return invalidFact()
	}
	return met
}

func retainEvidenceLessOrEq(left, right EvidenceState) bool {
	if !left.Valid() || !right.Valid() {
		return false
	}
	return left == right || left == EvidenceAbsent || right == EvidenceUnknown
}

func joinRetainEvidence(left, right EvidenceState) (EvidenceState, bool) {
	if !left.Valid() || !right.Valid() {
		return InvalidEvidenceState(), false
	}
	if left == EvidenceAbsent {
		return right, true
	}
	if right == EvidenceAbsent {
		return left, true
	}
	if left == right {
		return left, true
	}
	return EvidenceUnknown, true
}

func meetRetainEvidence(left, right EvidenceState) (EvidenceState, bool) {
	if !left.Valid() || !right.Valid() {
		return InvalidEvidenceState(), false
	}
	if left == EvidenceUnknown {
		return right, true
	}
	if right == EvidenceUnknown {
		return left, true
	}
	if left == right {
		return left, true
	}
	return EvidenceAbsent, true
}

func invalidFact() Fact {
	return Fact{Class: invalidPlacementResult, RetainEscape: InvalidEvidenceState()}
}

// RaiseClassChecked applies a non-aliasing placement requirement. It is used
// when a value must survive longer (for example across suspension) but the
// producer did not establish a second retained reference.
func RaiseClassChecked(current Fact, required Placement) (Fact, bool) {
	if _, ok := AuthenticateFactCell(current, true, true); !ok || !validAnalysisPlacement(required) || required == Bottom {
		return invalidFact(), false
	}
	class, ok := JoinChecked(current.Class, required)
	if !ok {
		return invalidFact(), false
	}
	return Fact{Class: class, RetainEscape: current.RetainEscape}, true
}

// RetainAtClassChecked applies an authenticated retaining operation whose
// placement consequence is already declaratively resolved. Unlike a control-
// flow join, the operation occurs on every path reaching this successor, so
// it establishes Proven even when the predecessor was Unknown.
func RetainAtClassChecked(current Fact, required Placement) (Fact, bool) {
	raised, ok := RaiseClassChecked(current, required)
	if !ok {
		return invalidFact(), false
	}
	raised.RetainEscape = EvidenceProven
	return raised, true
}

// ThroughContainerChecked propagates one contained allocation through the
// canonical state of its container. A definitely retained container retains
// every reachable child; an ambiguous container makes an otherwise unretained
// child ambiguous. A child already proven retained stays proven.
func ThroughContainerChecked(current, container Fact) (Fact, bool) {
	if _, ok := AuthenticateFactCell(current, true, true); !ok {
		return invalidFact(), false
	}
	if _, ok := AuthenticateFactCell(container, true, true); !ok {
		return invalidFact(), false
	}
	class, ok := JoinChecked(current.Class, container.Class)
	if !ok {
		return invalidFact(), false
	}
	retained := current.RetainEscape
	if retained != EvidenceProven {
		switch container.RetainEscape {
		case EvidenceProven:
			retained = EvidenceProven
		case EvidenceUnknown:
			retained = EvidenceUnknown
		case EvidenceRefuted:
		default:
			return invalidFact(), false
		}
	}
	return Fact{Class: class, RetainEscape: retained}, true
}
