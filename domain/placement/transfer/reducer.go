package transfer

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/placement"
)

// TransferFold is the one semantic reducer for an authenticated Transfer
// route. TransferSpec is strategy-neutral: it can authorize delivery, but it
// cannot itself publish a Placement fact. A route member exists only after
// the runtime delivery evidence was authenticated, and that member always
// carries the Send policy. Missing, reject-only, opaque, or malformed route
// evidence therefore never becomes Unknown here; it is no result/refusal.
func TransferFold(routeTag uint64, selected placement.Fact) (placement.Fact, structure.ReductionOutcome) {
	if routeTag == 0 || routeTag>>routeTagShift == 0 || routeTag&routeTagMask != uint64(placement.Send)+1 {
		return placement.BottomFact(), structure.Refuse
	}
	current, currentOK := placement.AuthenticateFactCell(selected, true, true)
	if !currentOK {
		return placement.BottomFact(), structure.Refuse
	}
	result, resultOK := placement.DisplaceFactChecked(current, placement.Send)
	if !resultOK {
		return placement.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
