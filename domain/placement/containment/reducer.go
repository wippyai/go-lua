package containment

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// ContainmentFold is the one containment judgment, and it is the judgment the
// rule runs: the placement of a parent allocation is joined into the cell of
// each allocation that parent's Heap value contains.
//
// Route construction decides WHICH child coordinates are addressed; it does
// not decide the placement policy. What the fold is handed is the two complete
// vectors the reads delivered, the coordinate this route publishes at, the tag
// that coordinate was correlated by, and the routed child cell itself. The tag
// is the parent/child coordinate pair the route owner packed, so which parent
// this child was reached from is decidable from the fold's own inputs rather
// than from a plan it would have to re-resolve.
//
// Invalid or unauthenticated evidence refuses. In particular this never turns
// an absent vector member into Unknown, and a parent whose Heap value is
// Bottom contains nothing and routes nothing.
func ContainmentFold(
	placements operand.SummaryVector[placementdomain.Fact],
	heaps operand.SummaryVector[heapdomain.Value],
	route heapdomain.Key,
	tag uint64,
	current placementdomain.Fact,
) (placementdomain.Fact, structure.ReductionOutcome) {
	if !placements.Valid() || !heaps.Valid() || !route.Valid() || route.Kind() != heapdomain.RootAllocation {
		return placementdomain.BottomFact(), structure.Refuse
	}
	parentIndex, parentIndexOK := containmentParentIndex(tag, placements.Count(), heaps.Count())
	if !parentIndexOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	parent, parentPresent, parentAvailable := placements.At(parentIndex)
	parent, parentOK := placementdomain.AuthenticateFactCell(parent, parentPresent, parentAvailable)
	if !parentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	child, childPresent, childAvailable := heaps.At(parentIndex)
	if !childAvailable || !childPresent {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := containmentValue(current, parent, child)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}

// containmentParentIndex recovers the parent coordinate a route tag was packed
// from. Both owners address dense coordinates, so the pair occupies one
// lossless tag; the vectors the reads delivered are the denominator it is
// bounded by.
func containmentParentIndex(tag uint64, placements, heaps int) (int, bool) {
	if tag == 0 || tag>>32 == 0 || tag&routeTagLowMask == 0 || placements < 0 || placements != heaps {
		return 0, false
	}
	parent64 := (tag >> 32) - 1
	if parent64 >= uint64(placements) {
		return 0, false
	}
	return int(parent64), true
}

// containmentValue is the placement policy itself: a child cell reached from a
// parent that holds it takes the parent's placement through the container.
//
// A parent with no placement policy, an unauthenticated child cell, and a
// parent Heap value that holds nothing are each evidence this judgment
// declines rather than widens.
func containmentValue(current, parent placementdomain.Fact, child heapdomain.Value) (placementdomain.Fact, bool) {
	if !validPlacement(current) || !validPlacement(parent) || !child.Valid() || child.IsBottom() {
		return placementdomain.BottomFact(), false
	}
	return placementdomain.ThroughContainerChecked(current, parent)
}
