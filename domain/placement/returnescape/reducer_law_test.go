package returnescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/placement"
)

func TestReturnEscapeFoldIsCanonicalAndRefusesMissingRouteEvidence(t *testing.T) {
	if _, outcome := ReturnEscapeFold(0, placement.DefaultFact()); outcome != structure.Refuse {
		t.Fatal("zero route tag was admitted")
	}
	if got, outcome := ReturnEscapeFold(1, placement.BottomFact()); outcome != structure.Refuse || got != placement.BottomFact() {
		t.Fatalf("Bottom predecessor = %s/%v, want refusal", got, outcome)
	}
	got, outcome := ReturnEscapeFold(1, placement.DefaultFact())
	want := placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("default predecessor = %s/%v, want %s/Concrete", got, outcome, want)
	}
	shared := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}
	got, outcome = ReturnEscapeFold(9, shared)
	want = placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("shared predecessor = %s/%v, want %s/Concrete", got, outcome, want)
	}
}
