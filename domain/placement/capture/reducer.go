package capture

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// CaptureFold is the one authored closure-capture judgment. The route tag is
// sealed route evidence; it selects a destination but does not alter the
// containment policy. Invalid evidence refuses and never widens a missing
// predecessor into Unknown.
func CaptureFold(parent placementdomain.Fact, routeTag uint64, current placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	if routeTag == 0 {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := placementdomain.ThroughContainerChecked(current, parent)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
