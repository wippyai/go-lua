package link_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

// These are deliberately separate from the corpus denominator gate: each
// keeps an individually runnable proof for a source shape that once exposed a
// missing Program-to-Link projection.
func TestRecurrenceExitArmRetainsCanonicalReadProjection(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	project, err := testfixture.FrozenCorpusProject("soundness/recurrence-exit-arm")
	if err != nil {
		t.Fatalf("fixture project: %v", err)
	}
	if _, err := testfixture.SealCorpusProject(contract, project); err != nil {
		t.Fatalf("seal recurrence-exit-arm through canonical Program-to-Link projection: %v", err)
	}
}

func TestBackwardGotoSealsCanonicalPackProjection(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	project, err := testfixture.FrozenCorpusProject("functions/goto-backward")
	if err != nil {
		t.Fatalf("fixture project: %v", err)
	}
	if _, err := testfixture.SealCorpusProject(contract, project); err != nil {
		t.Fatalf("seal goto-backward through canonical Program-to-Link projection: %v", err)
	}
}
