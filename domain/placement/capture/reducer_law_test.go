package capture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/placement"
)

func TestCaptureFoldUsesOnlyAuthenticatedParentAndRouteEvidence(t *testing.T) {
	if _, outcome := CaptureFold(placement.DefaultFact(), 0, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("zero Route tag was admitted")
	}
	if _, outcome := CaptureFold(placement.BottomFact(), 1, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("Bottom parent was admitted")
	}
	if _, outcome := CaptureFold(placement.DefaultFact(), 1, placement.BottomFact()); outcome != structure.Refuse {
		t.Fatal("Bottom child was admitted")
	}
	parent := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	child := placement.DefaultFact()
	got, outcome := CaptureFold(parent, 1, child)
	want := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("captured child = %s/%v, want %s/Concrete", got, outcome, want)
	}
}
