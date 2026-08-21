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

// Join is the least upper bound of the placement chain.
func Join(a, b Placement) Placement {
	left, leftOK := placementRank(a)
	right, rightOK := placementRank(b)
	if !leftOK || !rightOK {
		// Values outside the analysis vocabulary are not another point in the
		// chain. Refuse the operation at this boundary instead of allowing a
		// JIT/invalid value to acquire Unknown's rank and leak into the factor.
		return Unknown
	}
	if left > right {
		return a
	}
	return b
}

// Meet is the greatest lower bound of the placement chain.
func Meet(a, b Placement) Placement {
	left, leftOK := placementRank(a)
	right, rightOK := placementRank(b)
	if !leftOK || !rightOK {
		// There is no lattice result for an out-of-domain value. Unknown is the
		// fail-closed publication value; the invalid operand never participates
		// in a rank comparison.
		return Unknown
	}
	if left < right {
		return a
	}
	return b
}

// Widen equals Join because the placement lattice has finite height.
func Widen(prev, next Placement) Placement {
	return Join(prev, next)
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
