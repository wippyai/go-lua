package sourcecontrol

import "testing"

func TestOutcomeResumeAnchorRequiresAnExactOwner(t *testing.T) {
	var anchor OutcomeResumeAnchor
	if anchor.Available(nil) {
		t.Fatal("zero Outcome resume anchor reported availability")
	}
	if from, raw, direct, _, _, ok := anchor.Parts(nil); ok || from != 0 || raw != 0 || direct != 0 {
		t.Fatal("zero Outcome resume anchor exposed route terms")
	}
}
