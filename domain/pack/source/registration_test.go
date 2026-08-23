package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestRuleEntryDeclaresCanonicalSourceProgram(t *testing.T) {
	spec := RuleEntry()
	problem, ok := spec.Program.Check()
	if !ok {
		t.Fatalf("pack-source Program rejected: %+v", problem)
	}
	if spec.Program.JoinCount() != 0 || len(spec.Program.Fold.Inputs) != 0 || len(spec.Program.Fold.Outputs) != 1 {
		t.Fatalf("pack-source Program shape = joins:%d inputs:%d outputs:%d", spec.Program.JoinCount(), len(spec.Program.Fold.Inputs), len(spec.Program.Fold.Outputs))
	}
	if spec.Program.Carry != nil {
		t.Fatalf("pack-source carry = %#v", spec.Program.Carry)
	}
	if spec.Program.Fold.Outputs[0].Mode != program.ModeExact {
		t.Fatalf("pack-source output mode = %v", spec.Program.Fold.Outputs[0].Mode)
	}
}

// TestRoutedSourceOutputIsRefused is the nearest negative of the declaration
// above: a zero-read source rule derives no relation to route a write through,
// so the one degree of freedom that would let it write a coordinate it did not
// derive - a routed instead of exact output - must be refused by the program
// itself rather than by whatever binds it later.
func TestRoutedSourceOutputIsRefused(t *testing.T) {
	declaration := RuleEntry().Program
	declaration.Fold.Outputs[0].Mode = program.ModeRoute
	problem, ok := declaration.Check()
	if ok || problem.Kind != program.ProblemOutput {
		t.Fatalf("routed pack-source output admitted: problem=%+v ok=%t", problem, ok)
	}
}
