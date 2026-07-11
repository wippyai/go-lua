package diffharness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/diffharness"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

func solveUnit(t *testing.T, source string) diffharness.UnitResult {
	t.Helper()
	result := checktest.CheckFileAndExport(source, "unit", "unit.lua")
	encoded, err := manifest.Encode(result.Manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	return diffharness.UnitResult{
		Diagnostics:   result.Errors,
		Manifest:      encoded,
		RenderOptions: diagnostic.RenderOptions{Sources: diagnostic.SourceMap{"unit.lua": source}},
	}
}

func TestDiffIdenticalSolvesAreEmpty(t *testing.T) {
	source := "local value: number = 1\nreturn value\n"
	before := solveUnit(t, source)
	after := solveUnit(t, source)
	if got := diffharness.Diff(before, after); len(got) != 0 {
		t.Fatalf("identical solve diff = %s, want empty", got)
	}
}

func TestDiffPerturbedSolveIsStableJSONL(t *testing.T) {
	before := solveUnit(t, "local value: number = 1\nreturn value\n")
	after := solveUnit(t, "local value: number = \"wrong\"\nreturn \"changed\"\n")
	first := diffharness.Diff(before, after)
	second := diffharness.Diff(before, after)
	if len(first) == 0 {
		t.Fatal("perturbed solve diff is empty")
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("diff is not stable\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.Contains(string(first), `"kind":"diagnostic"`) || !strings.Contains(string(first), `"kind":"manifest"`) {
		t.Fatalf("diff = %s, want diagnostic and manifest records", first)
	}
}
