package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestOperationSubedgeRelationProjectsOnlySealedCoordinates(t *testing.T) {
	operation := protectedSubedgeOperation("neutral-subedge-relation", false, false, false)
	for index := range operation.Outcomes {
		operation.Outcomes[index].Values.Fixed = []schematype.Type{testAny}
	}
	operation.SubedgeRelation = &SubedgeRelationSpec{
		Operand: 1, Selector: 37, Subedge: 1, ResultOutcome: 0, Result: 0,
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{operation}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"neutral-subedge-relation"}})
	if !ok {
		t.Fatal("operation missing")
	}
	operand, selector, subedge, outcome, result, ok := contract.OperationSubedgeRelation(op)
	if !ok || operand != 1 || selector != 37 || subedge == 0 || result != 0 {
		t.Fatalf("relation = %d/%d/%d/%d/%d/%v", operand, selector, subedge, outcome, result, ok)
	}
	if got, ok := contract.OperationSubedgeRelationOutcome(op, flowkind.OutcomeNormal); !ok || got != outcome {
		t.Fatalf("normal outcome = %d/%v", got, ok)
	}
	if count := contract.OperationSubedgeRelationEffectAliasCount(op); count != 0 {
		t.Fatalf("effect aliases = %d, want 0", count)
	}
}
