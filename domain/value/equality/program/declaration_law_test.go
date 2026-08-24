package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestBinaryEqualityProgramDeclaresTheTwoExactReadsAndIdentityCarry(t *testing.T) {
	declaration := BinaryEquality()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("binary equality declaration rejected: %#v", problem)
	}
	if declaration.Candidate.Member != BinaryEqualityCandidates || declaration.JoinCount() != 2 {
		t.Fatalf("candidate/joins = %#v/%d", declaration.Candidate, declaration.JoinCount())
	}
	for index := 0; index < declaration.JoinCount(); index++ {
		join, joinOK := declaration.JoinAt(index)
		if !joinOK || len(join.Sources) != 1 || !join.Sources[0].Candidate || join.Relation.Member != BinaryEqualitySources || join.Read.Input != 0 || join.Read.Axis.EntryReference() != declaration.Candidate.Axis || join.Read.Form != ruleprogram.Exact || join.Read.Contract.Order != ruleprogram.OrderCanonical || join.Read.Contract.Sparse != ruleprogram.SparseExplicit || join.Read.Contract.OnOpaque != ruleprogram.OnOpaqueRefuse || join.Read.Contract.Multiplicity != ruleprogram.MultiplicityOne {
			t.Fatalf("join[%d] = %#v", index, join)
		}
	}
	if declaration.Fold.Reducer.Member != BinaryEqualityReducer || len(declaration.Fold.Inputs) != 2 || declaration.Fold.Inputs[0] != 0 || declaration.Fold.Inputs[1] != 1 || len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("fold = %#v", declaration.Fold)
	}
	output := declaration.Fold.Outputs[0]
	if output.Column.Key != OutputKey || output.Destination.Member != BinaryEqualityWrite || output.Mode != ruleprogram.ModeExact || output.ValueSlot != 0 {
		t.Fatalf("output = %#v", output)
	}
	if declaration.Carry == nil || declaration.Carry.Input != 0 || declaration.Carry.Mode != ruleprogram.CarryIdentity || declaration.Carry.Transform.Declared() {
		t.Fatalf("carry = %#v", declaration.Carry)
	}
}

func TestEqualityAliasReturnsTheSameProgram(t *testing.T) {
	first := BinaryEquality()
	second := Equality()
	if first.Digest() != second.Digest() {
		t.Fatalf("program aliases have different content identities")
	}
}

func TestEqualityRuleEntryAdmitsItsProgram(t *testing.T) {
	entry := RuleEntry()
	template, ok := rule.New(entry)
	if !ok || template == nil || template.Key() != RuleKey {
		t.Fatalf("equality rule entry = %v/%t", template, ok)
	}
}
