package front_test

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
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

func TestCompileBodyAssignmentReadsShareStatementSnapshot(t *testing.T) {
	artifact, err := front.CompileBody(`
local left, right = 1, 2
left, right = right, left
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	var writes []equation.Equation
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind == "environment-write" {
			writes = append(writes, operation)
		}
	}
	if len(writes) != 4 {
		t.Fatalf("assignment equations = %d, want 4", len(writes))
	}
	readBoundary := func(operation equation.Equation) string {
		t.Helper()
		for _, operand := range operation.Operands {
			if operand.Role == "read-before" {
				return string(operand.Term.Encoding)
			}
		}
		t.Fatalf("assignment %s has no read-before operand", operation.Target.Name)
		return ""
	}
	if got, want := readBoundary(writes[2]), readBoundary(writes[3]); got != want {
		t.Fatalf("parallel assignment read boundaries = %q and %q, want one pre-write snapshot", got, want)
	}
	if got := readBoundary(writes[2]); got == readBoundary(writes[1]) {
		t.Fatalf("second assignment statement reused its prior statement boundary %q", got)
	}
}

func TestCompileBodyRejectsUnsupportedInstructionWithoutArtifact(t *testing.T) {
	artifact, err := front.CompileBody(`local value = source()`) // calls are the call-results family, not environment-write.
	if !errors.Is(err, front.ErrUnsupportedInstruction) {
		t.Fatalf("CompileBody error = %v, want unsupported instruction", err)
	}
	if len(artifact.Equations) != 0 {
		t.Fatalf("CompileBody returned %d equations with a rejected instruction", len(artifact.Equations))
	}
}
