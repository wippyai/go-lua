package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestRuleEntryDeclaresCanonicalTypeFactTransferProgram(t *testing.T) {
	spec := RuleEntry()
	problem, ok := spec.Program.Check()
	if !ok {
		t.Fatalf("static-transfer Program rejected: %+v", problem)
	}
	if spec.Program.JoinCount() != 1 || len(spec.Program.Fold.Inputs) != 1 || len(spec.Program.Fold.Outputs) != 1 {
		t.Fatalf("static-transfer Program shape = joins:%d inputs:%d outputs:%d", spec.Program.JoinCount(), len(spec.Program.Fold.Inputs), len(spec.Program.Fold.Outputs))
	}
	join, _ := spec.Program.JoinAt(0)
	if join.Read.Input != 0 || join.Read.Form != program.Exact || join.Read.Contract.Order != program.OrderCanonical ||
		join.Read.Contract.Sparse != program.SparseExplicit || join.Read.Contract.OnOpaque != program.OnOpaqueRefuse ||
		join.Read.Contract.Multiplicity != program.MultiplicityOne || join.Read.Contract.DenominatorRef.Declared() {
		t.Fatalf("static-transfer exact read contract = %+v", join.Read)
	}
	if spec.Program.Carry == nil || spec.Program.Carry.Input != 0 || spec.Program.Carry.Mode != program.CarryIdentity || spec.Program.Carry.Transform.Declared() {
		t.Fatalf("static-transfer carry = %#v", spec.Program.Carry)
	}
}
