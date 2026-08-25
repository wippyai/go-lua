package publicationescape

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// PublicationEscapeFold is the one irreducible Placement judgment for an
// authenticated publication route.  The route relation owns whether its
// requirement came from an exact, open, or opaque Effect/Value subject; this
// reducer only applies that already-authenticated requirement to the selected
// Placement cell.
func PublicationEscapeFold(requirement placementdomain.Placement, current placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	current, currentOK := placementdomain.AuthenticateFactCell(current, true, true)
	if !currentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	if requirement == placementdomain.Unknown {
		return placementdomain.UnknownFact(), structure.Concrete
	}
	var escape placementdomain.Escape
	switch requirement {
	case placementdomain.OwnedHeap:
		escape = placementdomain.Retain
	case placementdomain.SharedHeap:
		escape = placementdomain.Send
	default:
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, ok := placementdomain.DisplaceFactChecked(current, escape)
	if !ok {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
