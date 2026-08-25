package ingress

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestRuleEntryDeclaresCanonicalSourceProgram(t *testing.T) {
	spec := RuleEntry()
	problem, ok := spec.Program.Check()
	if !ok {
		t.Fatalf("heap-ingress Program rejected: %+v", problem)
	}
	if spec.Program.JoinCount() != 0 || len(spec.Program.Fold.Inputs) != 0 || len(spec.Program.Fold.Outputs) != 1 {
		t.Fatalf("heap-ingress Program shape = joins:%d inputs:%d outputs:%d", spec.Program.JoinCount(), len(spec.Program.Fold.Inputs), len(spec.Program.Fold.Outputs))
	}
	if spec.Program.Carry != nil {
		t.Fatalf("heap-ingress carry = %#v", spec.Program.Carry)
	}
	if spec.Program.Fold.Outputs[0].Mode != program.ModeExact {
		t.Fatalf("heap-ingress output mode = %v", spec.Program.Fold.Outputs[0].Mode)
	}
}
