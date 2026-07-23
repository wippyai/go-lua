package front_test

import (
	"errors"
	"strings"
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

func TestCompileBodyLowersPathStoreInvalidationAndIndexMutationTogether(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "id"
record.label = false
record[key].state = nil
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	got := make(map[string][]equation.Equation)
	for _, equation := range artifact.Equations {
		got[equation.Occurrence.Kind] = append(got[equation.Occurrence.Kind], equation)
	}
	for kind, want := range map[string]int{
		"entry": 1, "environment-write": 1, "path-replacement": 1,
		"path-invalidation": 1, "index-mutation": 1,
	} {
		if count := len(got[kind]); count != want {
			t.Fatalf("%s occurrences = %d, want %d; all = %#v", kind, count, want, got)
		}
	}
	static := operands(got["path-replacement"][0])
	if static["value"] != "scalar/bool/false" {
		t.Fatalf("static replacement value = %q, want false", static["value"])
	}
	invalidation := operands(got["path-invalidation"][0])
	if invalidation["key"] == "" {
		t.Fatalf("invalidation lost the dynamic key: %#v", invalidation)
	}
	if invalidation["suffix"] != "suffix/.state" {
		t.Fatalf("invalidation suffix = %q, want suffix/.state", invalidation["suffix"])
	}
	mutation := operands(got["index-mutation"][0])
	if mutation["value"] != "scalar/nil" {
		t.Fatalf("index mutation value = %q, want scalar/nil", mutation["value"])
	}
	if mutation["suffix"] != invalidation["suffix"] || mutation["key"] != invalidation["key"] {
		t.Fatalf("paired path operations disagree: invalidation=%#v mutation=%#v", invalidation, mutation)
	}
}

func TestCompileBodyGuardsBothHalvesOfDynamicIndexWrite(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "id"
if true then
    record[key] = 3
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "path-invalidation" && equation.Occurrence.Kind != "index-mutation" {
			continue
		}
		if len(equation.Guards) != 1 {
			t.Fatalf("%s guards = %#v, want exactly the selected branch edge", equation.Occurrence.Kind, equation.Guards)
		}
	}
}

func TestCompileBodyLowersDynamicIndexReadWithoutTreatingNilAsAbsence(t *testing.T) {
	artifact, err := front.CompileBody(`
local key = "missing"
local result = record[key]
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, equation := range artifact.Equations {
		if equation.Occurrence.Kind != "path-replacement" {
			continue
		}
		got := operands(equation)
		if got["container"] == "" || got["key"] == "" || got["target"] == "" {
			t.Fatalf("dynamic read omitted an operand: %#v", got)
		}
		if _, hasValue := got["value"]; hasValue {
			t.Fatalf("dynamic read invented a replacement value instead of preserving lookup semantics: %#v", got)
		}
		return
	}
	t.Fatal("missing path replacement for dynamic index read")
}

func operands(equation equation.Equation) map[string]string {
	result := make(map[string]string, len(equation.Operands))
	for _, operand := range equation.Operands {
		result[operand.Role] = string(operand.Term.Encoding)
	}
	return result
}

func TestCompileBodyLowersNormalizedBranchPredicate(t *testing.T) {
	artifact, err := front.CompileBody(`
local status = "ready"
if status == "ready" then
    local result = "selected"
end
`)
	if err != nil {
		t.Fatalf("CompileBody: %v", err)
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "predicate" {
				if !strings.HasPrefix(string(operand.Term.Encoding), "front/branch-predicate/v1/") {
					t.Fatalf("predicate encoding = %q", operand.Term.Encoding)
				}
				return
			}
		}
		t.Fatal("normalized branch had no predicate operand")
	}
	t.Fatal("artifact had no branch relation")
}

func TestCompileBodyRejectsBranchWithoutACompleteSelector(t *testing.T) {
	_, err := front.CompileBody(`
local function predicate()
    return true
end
if predicate() then
    local selected = true
end
`)
	if err == nil {
		t.Fatal("CompileBody accepted a branch whose call result family is not lowered")
	}
	if !strings.Contains(err.Error(), "unsupported WIR instruction") {
		t.Fatalf("CompileBody error = %v, want unsupported instruction", err)
	}
}
