package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestPublicationEscapeFoldMapsOnlyAuthenticatedRouteRequirements(t *testing.T) {
	owned, outcome := PublicationEscapeFold(placementdomain.OwnedHeap, placementdomain.DefaultFact())
	if outcome != structure.Concrete || owned.Class != placementdomain.OwnedHeap || owned.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("owned=%v outcome=%v", owned, outcome)
	}
	shared, outcome := PublicationEscapeFold(placementdomain.SharedHeap, placementdomain.DefaultFact())
	if outcome != structure.Concrete || shared.Class != placementdomain.SharedHeap || shared.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("shared=%v outcome=%v", shared, outcome)
	}
	unknown, outcome := PublicationEscapeFold(placementdomain.Unknown, placementdomain.DefaultFact())
	if outcome != structure.Concrete || unknown != placementdomain.UnknownFact() {
		t.Fatalf("unknown=%v outcome=%v", unknown, outcome)
	}
	if refused, outcome := PublicationEscapeFold(placementdomain.Stack, placementdomain.DefaultFact()); outcome != structure.Refuse || refused != placementdomain.BottomFact() {
		t.Fatalf("invalid requirement=%v outcome=%v", refused, outcome)
	}
}

func TestPublicationEscapeFoldRefusesUnauthenticatedCurrentBeforeUnknown(t *testing.T) {
	for name, current := range map[string]placementdomain.Fact{
		"bottom":  placementdomain.BottomFact(),
		"invalid": {Class: placementdomain.Placement(255), RetainEscape: placementdomain.EvidenceRefuted},
	} {
		t.Run(name, func(t *testing.T) {
			result, outcome := PublicationEscapeFold(placementdomain.Unknown, current)
			if outcome != structure.Refuse || result != placementdomain.BottomFact() {
				t.Fatalf("result=%v outcome=%v, want Bottom/Refuse", result, outcome)
			}
		})
	}
}
