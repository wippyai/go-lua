package placement

import (
	"github.com/wippyai/go-lua/analysis/lattice"
)

func Lattice() lattice.Lattice[Placement] {
	return lattice.Lattice[Placement]{
		Bottom:   func() Placement { return Bottom },
		Top:      func() Placement { return Unknown },
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
	}
}

// The chain is:
//
//	bottom < stack < owned-heap < shared-heap < unknown
//
// Higher points are less precise and more conservative. Joining any path that
// requires a more shared placement moves the result upward; Unknown is Top.
// LessOrEq reports whether b conservatively covers a.
func LessOrEq(a, b Placement) bool {
	left, leftOK := placementRank(a)
	right, rightOK := placementRank(b)
	return leftOK && rightOK && left <= right
}

// JoinChecked is the least upper bound of the Placement chain. The checked
// form is the producer-facing API: invalid/JIT values are rejected instead
// of being turned into Unknown, because Unknown is a real semantic top.
func JoinChecked(a, b Placement) (Placement, bool) {
	left, leftOK := placementRank(a)
	right, rightOK := placementRank(b)
	if !leftOK || !rightOK {
		return invalidPlacementResult, false
	}
	if left > right {
		return a, true
	}
	return b, true
}

// Join is the lattice callback spelling of JoinChecked. Lattice callbacks
// cannot return an error, so an invalid input produces the private invalid
// sentinel. It is intentionally not Unknown and is rejected by every
// producer-facing checked seam before publication.
func Join(a, b Placement) Placement {
	joined, ok := JoinChecked(a, b)
	if !ok {
		return invalidPlacementResult
	}
	return joined
}

// MeetChecked is the checked greatest lower bound of the Placement chain.
func MeetChecked(a, b Placement) (Placement, bool) {
	left, leftOK := placementRank(a)
	right, rightOK := placementRank(b)
	if !leftOK || !rightOK {
		return invalidPlacementResult, false
	}
	if left < right {
		return a, true
	}
	return b, true
}

// Meet is the lattice callback spelling of MeetChecked. Invalid input is
// represented by the private invalid sentinel, never by Unknown.
func Meet(a, b Placement) Placement {
	met, ok := MeetChecked(a, b)
	if !ok {
		return invalidPlacementResult
	}
	return met
}

// WidenChecked equals JoinChecked because the Placement lattice has finite
// height.
func WidenChecked(prev, next Placement) (Placement, bool) {
	return JoinChecked(prev, next)
}

// Widen is the lattice callback spelling of WidenChecked.
func Widen(prev, next Placement) Placement {
	widened, ok := WidenChecked(prev, next)
	if !ok {
		return invalidPlacementResult
	}
	return widened
}

// Equal is lattice equivalence.
func Equal(a, b Placement) bool {
	_, leftOK := placementRank(a)
	_, rightOK := placementRank(b)
	return leftOK && rightOK && a == b
}

// placementRank admits only the declarative Placement axis. JIT-only and
// invalid values deliberately have no rank: treating either as Unknown would
// make an out-of-domain value appear to be a valid analysis fact.
func placementRank(v Placement) (int, bool) {
	switch v {
	case Bottom:
		return 0, true
	case Stack:
		return 1, true
	case OwnedHeap:
		return 2, true
	case SharedHeap:
		return 3, true
	case Unknown:
		return 4, true
	default:
		return 0, false
	}
}

// invalidPlacementResult is deliberately outside the analysis vocabulary.
// Placement has no error value in the generic lattice callback interface, so
// this sentinel preserves failure without manufacturing a semantic Unknown
// fact. Checked producer APIs must be used whenever invalid input is possible.
const invalidPlacementResult Placement = ^Placement(0)
