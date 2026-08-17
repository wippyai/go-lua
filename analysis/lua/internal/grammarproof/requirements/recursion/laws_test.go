package recursion

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGrammarRecursionHasBaseAndStepPremises(t *testing.T) {
	root := moduleRoot(t)
	report, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Logf("grammar induction: required=%d missing=%d", report.RequiredCount(), report.MissingCount())
}

// moduleRoot walks up from this test source until it finds the directory that
// owns go.mod. Anchoring on the module marker keeps the proof independent of
// where the grammarproof tree sits inside the module.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate grammar recursion source")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root: no go.mod above test file")
		}
		root = parent
	}
}
