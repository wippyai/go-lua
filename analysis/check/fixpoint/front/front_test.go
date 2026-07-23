package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

func TestCompileBodyLowersScalarAssignmentAndBranchSlice(t *testing.T) {
	artifact, err := front.CompileBody(`
local first = 1
local second = first
if true then
    local third = second
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	if artifact.CanonicalBytes() == nil {
		t.Fatal("CompileBody returned a non-canonical artifact")
	}
	got := make(map[string]int)
	for _, equation := range artifact.Equations {
		got[equation.Occurrence.Kind]++
	}
	if got["entry"] != 1 || got["environment-write"] != 3 || got["branch-relations"] != 1 {
		t.Fatalf("lowered occurrence kinds = %#v", got)
	}
}
