package formal

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/placement"
)

// FormalFold is the one semantic reducer for an authenticated formal route.
// Route planning chooses and authenticates the route member; this fold only
// interprets the sealed route tag and applies Placement's canonical
// displacement. An open/opaque route is allowed to publish Unknown only
// because its producer authenticated that widening in the formal semantics.
// Missing, zero, or malformed route evidence is refusal, never a fabricated
// Placement result.
func FormalFold(routeTag uint64, selected placement.Fact) (placement.Fact, structure.ReductionOutcome) {
	if routeTag == 0 || routeTag>>routeTagShift == 0 {
		return placement.BottomFact(), structure.Refuse
	}
	current, currentOK := placement.AuthenticateFactCell(selected, true, true)
	if !currentOK {
		return placement.BottomFact(), structure.Refuse
	}
	code := routeTag & routeTagMask
	if code == routeCodeUnknown {
		return placement.UnknownFact(), structure.Concrete
	}
	if code == 0 || code > uint64(placement.Return)+1 {
		return placement.BottomFact(), structure.Refuse
	}
	escape := placement.Escape(code - 1)
	result, resultOK := placement.DisplaceFactChecked(current, escape)
	if !resultOK {
		return placement.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
