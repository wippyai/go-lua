package typestate

import "testing"

func TestObligationHasOneCanonicalStateSetRepresentation(t *testing.T) {
	left, leftOK := NewObligation("rolled_back", "committed", "committed")
	right, rightOK := NewObligation("committed", "rolled_back")
	if !leftOK || !rightOK || left != right {
		t.Fatalf("equivalent final-state sets did not produce one comparable obligation: %#v/%v %#v/%v", left, leftOK, right, rightOK)
	}
	states := left.FinalStateList()
	if len(states) != 2 || states[0] != "committed" || states[1] != "rolled_back" {
		t.Fatalf("canonical final-state order = %v", states)
	}
	if !left.SatisfiedBy("committed") || left.SatisfiedBy("active") || left.Empty() {
		t.Fatal("canonical obligation membership is inconsistent")
	}
}

func TestObligationRefusesEmptyStateMember(t *testing.T) {
	if obligation, ok := NewObligation("finished", ""); ok || !obligation.Empty() {
		t.Fatalf("malformed obligation admitted: %#v/%v", obligation, ok)
	}
	empty, ok := NewObligation()
	if !ok || !empty.Empty() || len(empty.FinalStateList()) != 0 {
		t.Fatalf("empty obligation = %#v/%v", empty, ok)
	}
}
