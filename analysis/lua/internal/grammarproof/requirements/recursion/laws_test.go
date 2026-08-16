package recursion

import (
	"path/filepath"
	"testing"
)

func TestGrammarRecursionHasBaseAndStepPremises(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	report, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Logf("grammar induction: required=%d missing=%d", report.RequiredCount(), report.MissingCount())
}
