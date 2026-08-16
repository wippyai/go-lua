package vocabulary

import "testing"

func TestBundleIsClosedDistinctAndReplayable(t *testing.T) {
	first, firstOK := New()
	second, secondOK := New()
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatal("global semantic vocabulary unavailable")
	}
	if first != second {
		t.Fatal("equivalent global semantic vocabulary did not replay exactly")
	}
	if first.ValueFactor == first.CallFactor || first.ValueSourceRule.Rule == first.ValueSourceRule.Operand {
		t.Fatal("distinct semantic roles collided")
	}
	if _, ok := Key(""); ok {
		t.Fatal("empty semantic role was accepted")
	}
}
