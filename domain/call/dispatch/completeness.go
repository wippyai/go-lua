package dispatch

import (
	"math"

	calldomain "github.com/wippyai/go-lua/domain/call"
)

// Completeness is Call dispatch's closed conclusion about the target image at
// one application. Unknown intentionally carries no cardinality: the opaque
// arm is not a finite target. Complete is reserved for a proven singleton;
// finite multi-target images remain Incomplete for native specialization.
type Completeness uint8

const (
	InvalidCompleteness Completeness = iota
	Unknown
	Incomplete
	Complete
)

func (value Completeness) Valid() bool {
	return value >= Unknown && value <= Complete
}

// CalleeSet is the immutable scalar issued from Call's final lattice cell.
// Cardinality is present only for a finite target image.
type CalleeSet struct {
	completeness Completeness
	cardinality  uint32
	finite       bool
}

func (fact CalleeSet) Available() bool {
	if !fact.completeness.Valid() {
		return false
	}
	if fact.completeness == Unknown {
		return !fact.finite && fact.cardinality == 0
	}
	return fact.finite && fact.cardinality > 0
}

func (fact CalleeSet) Completeness() Completeness {
	if !fact.Available() {
		return InvalidCompleteness
	}
	return fact.completeness
}

func (fact CalleeSet) Cardinality() (uint32, bool) {
	return fact.cardinality, fact.Available() && fact.finite
}

// ClassifyCalleeSet consumes only Call's final owner-issued value. It does not
// inspect Program syntax, Value atoms, activation routes, or target topology.
// Bottom is absence of a conclusion and is withheld rather than published as
// a zero-cardinality fact.
func ClassifyCalleeSet(value calldomain.Value) (CalleeSet, bool) {
	return classifyCalleeSetState(
		value.IsTop(),
		value.IsOpen(),
		value.IsComplete(),
		value.KnownTargetCount(),
	)
}

func classifyCalleeSetState(top, open, complete bool, count int) (CalleeSet, bool) {
	if count < 0 || uint64(count) > math.MaxUint32 {
		return CalleeSet{}, false
	}
	switch {
	case top && !open && !complete:
		if count != 0 {
			return CalleeSet{}, false
		}
		return CalleeSet{completeness: Unknown}, true
	case open && !top && !complete:
		return CalleeSet{completeness: Unknown}, true
	case complete && !top && !open:
		if count == 0 {
			return CalleeSet{}, false
		}
		completeness := Incomplete
		if count == 1 {
			completeness = Complete
		}
		fact := CalleeSet{completeness: completeness, cardinality: uint32(count), finite: true}
		return fact, fact.Available()
	default:
		return CalleeSet{}, false
	}
}
