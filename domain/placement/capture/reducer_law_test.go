package capture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

func TestCaptureFoldUsesOnlyAuthenticatedParentAndRouteEvidence(t *testing.T) {
	route := captureLawRoute(t)
	if _, outcome := CaptureFold(placement.DefaultFact(), route, 0, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("zero Route tag was admitted")
	}
	// A coordinate this owner never issued, and one that is not an allocation
	// root, are both route evidence the fold refuses rather than publishes at.
	if _, outcome := CaptureFold(placement.DefaultFact(), heapdomain.Key{}, 1, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("unissued Route coordinate was admitted")
	}
	if _, outcome := CaptureFold(placement.BottomFact(), route, 1, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("Bottom parent was admitted")
	}
	if _, outcome := CaptureFold(placement.DefaultFact(), route, 1, placement.BottomFact()); outcome != structure.Refuse {
		t.Fatal("Bottom child was admitted")
	}
	parent := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	child := placement.DefaultFact()
	got, outcome := CaptureFold(parent, route, 1, child)
	want := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("captured child = %s/%v, want %s/Concrete", got, outcome, want)
	}
}
