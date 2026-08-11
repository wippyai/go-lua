package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func TestSolverOwnsLinkBySealedContent(t *testing.T) {
	programValue := localLawProgram(t, `return 1`)
	owned, _ := localLawLink(t, programValue)
	equalContent, _ := localLawLink(t, programValue)
	foreignProgram := localLawProgram(t, `return 2`)
	foreign, _ := localLawLink(t, foreignProgram)

	if owned == equalContent {
		t.Fatal("test setup reused the same Link pointer")
	}
	if owned.ContentID() != equalContent.ContentID() {
		t.Fatal("independently sealed equal Links have different content identity")
	}
	if owned.ContentID() == foreign.ContentID() {
		t.Fatal("different Links have equal content identity")
	}

	solver, err := engine.New(owned)
	if err != nil {
		t.Fatalf("New Solver: %v", err)
	}
	if !solver.OwnsLink(owned) {
		t.Fatal("Solver rejected its source Link")
	}
	if !solver.OwnsLink(equalContent) {
		t.Fatal("Solver rejected an independently sealed equal Link")
	}
	if solver.OwnsLink(foreign) {
		t.Fatal("Solver accepted a different Link")
	}
	if solver.OwnsLink(nil) {
		t.Fatal("Solver accepted nil Link")
	}
}
