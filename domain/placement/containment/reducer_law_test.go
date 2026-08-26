package containment

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"testing"
)

func TestContainmentFoldUsesOnlyAuthenticatedPlacementFacts(t *testing.T) {
	result, outcome := ContainmentFold(placementdomain.DefaultFact(), placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven})
	if outcome != structure.Concrete || result.Class != placementdomain.OwnedHeap || result.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("result=%v outcome=%v", result, outcome)
	}
	if result, outcome := ContainmentFold(placementdomain.BottomFact(), placementdomain.DefaultFact()); outcome != structure.Refuse || result != placementdomain.BottomFact() {
		t.Fatalf("invalid current result=%v outcome=%v", result, outcome)
	}
}
