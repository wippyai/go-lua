package containment

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// ContainmentFold is the one irreducible containment judgment.  Route
// construction decides which child coordinates are addressed; it does not
// decide the placement policy.  The engine supplies the authenticated child
// cell and its parent cell after the complete Placement/Heap vector reads have
// admitted the route.
//
// Invalid or unauthenticated evidence refuses.  In particular, this function
// never turns an absent vector member into Unknown.
func ContainmentFold(current, parent placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	result, ok := placementdomain.ThroughContainerChecked(current, parent)
	if !ok {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
