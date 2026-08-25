// Package birth owns Placement's irreducible allocation-birth judgment.
// Producer selection remains Value-owned; this package only turns an
// authenticated, present allocation fact into Placement's initial fact.
package birth

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func initial() (placementdomain.Fact, structure.ReductionOutcome) {
	return placementdomain.DefaultFact(), structure.Concrete
}

// Fresh publishes the initial Placement fact after Value has published the
// exact fresh-result cell. The input's presence is the existence proof; the
// reducer deliberately does not reinterpret its Value alternatives.
func Fresh(candidate valuedomain.FreshResultCall, _ valuedomain.Value) (placementdomain.Fact, structure.ReductionOutcome) {
	if _, ok := candidate.Key(); !ok {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return initial()
}

// Allocation publishes the same initial fact after Value has published an
// authored allocation result. The receipt is the owner-issued candidate and
// its exact Value cell is the existence proof.
func Allocation(candidate *valuedomain.AllocationResult, _ valuedomain.Value) (placementdomain.Fact, structure.ReductionOutcome) {
	if _, ok := candidate.Key(); !ok {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return initial()
}
