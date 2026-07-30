package placement

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

func Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   func() Value { return Bottom },
		Top:      func() Value { return Unknown },
		Equal:    Equal,
		LessOrEq: LessOrEq,
		Join:     Join,
		Meet:     Meet,
		Widen:    Widen,
	}
}

// Value is a compatibility alias for the canonical allocation-placement
// vocabulary. New in-memory code should name Placement directly.
//
// The chain is:
//
//	bottom < stack < owned-heap < shared-heap < unknown
//
// Higher points are less precise and more conservative. Joining any path that
// requires a more shared placement moves the result upward; Unknown is Top.
type Value = Placement

// LessOrEq reports whether b conservatively covers a.
func LessOrEq(a, b Value) bool {
	return placementRank(a) <= placementRank(b)
}

// Join is the least upper bound of the placement chain.
func Join(a, b Value) Value {
	if placementRank(a) > placementRank(b) {
		return a
	}
	return b
}

// Meet is the greatest lower bound of the placement chain.
func Meet(a, b Value) Value {
	if placementRank(a) < placementRank(b) {
		return a
	}
	return b
}

// Widen equals Join because the placement lattice has finite height.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence.
func Equal(a, b Value) bool {
	return a == b
}

func placementRank(v Value) int {
	switch v {
	case Bottom:
		return 0
	case Stack:
		return 1
	case OwnedHeap:
		return 2
	case SharedHeap:
		return 3
	case Unknown:
		return 4
	default:
		// JIT-only and invalid placements are outside this lattice. Keep them
		// conservative if one crosses the analysis boundary.
		return 4
	}
}
