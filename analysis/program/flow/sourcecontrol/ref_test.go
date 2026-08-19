package sourcecontrol

import "testing"

func TestOpaqueReferencesFailClosedWithoutTheirIssuingGraph(t *testing.T) {
	var node NodeRef
	var phase PhaseRef
	var arc ArcRef
	if node.Available() || phase.Available() || arc.Available() || phase.OutcomePhase() {
		t.Fatal("zero sourcecontrol reference reported availability")
	}
	if SameOwner(node, NodeRef{}) || SamePhaseOwner(phase, PhaseRef{}) || SameArcRef(arc, ArcRef{}) {
		t.Fatal("zero sourcecontrol references compared as owned")
	}
	if ref, ok := (&Result{}).ArcRefAt(0); ok || ref.Available() {
		t.Fatal("unavailable graph issued an ArcRef")
	}
	var result Result
	owner := result.Owner()
	if owner.Available() || result.OwnsOwner(owner) {
		t.Fatal("unavailable sourcecontrol Result exposed an Owner")
	}
}
