package capture

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// CaptureFold is the one closure-capture judgment, and it is the judgment the
// rule runs: for the allocation one route names, the closure's own placement
// is joined into that allocation's cell.
//
// The route evidence is authenticated here rather than beside the call. A
// selection hands the fold the destination coordinate it publishes at and the
// tag that coordinate was correlated by, so what a route names is decidable
// from the fold's own inputs: a zero tag is not an owner-issued route member,
// and a coordinate that is not a root allocation is not a captured allocation.
// Invalid evidence refuses and never widens a missing predecessor into
// Unknown.
func CaptureFold(parent placementdomain.Fact, route heapdomain.Key, tag uint64, current placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	if tag == 0 || !route.Valid() || route.Kind() != heapdomain.RootAllocation {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := placementdomain.ThroughContainerChecked(current, parent)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
