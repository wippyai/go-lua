package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestRelationIdentityQueriesRoundTripFormalAndOutcomeRows(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("relation-id", []OutcomeSpec{{
		Kind:   flowkind.OutcomeNormal,
		Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
	}})}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"relation-id"}})
	if !ok {
		t.Fatal("relation identity operation missing")
	}
	formal, ok := contract.InputFormalID(op, 0)
	if !ok || !formal.Available() {
		t.Fatalf("InputFormalID = %v/%v", formal, ok)
	}
	if owner, coordinate, ok := contract.FindInputFormalID(formal); !ok || owner != op || coordinate != 0 {
		t.Fatalf("FindInputFormalID = %d/%d/%v", owner, coordinate, ok)
	}
	result, ok := contract.OutcomeResultID(op, 0, 0)
	if !ok || !result.Available() {
		t.Fatalf("OutcomeResultID = %v/%v", result, ok)
	}
	if owner, outcome, ordinal, ok := contract.FindOutcomeResultID(result); !ok || owner != op || outcome != 0 || ordinal != 0 {
		t.Fatalf("FindOutcomeResultID = %d/%d/%d/%v", owner, outcome, ordinal, ok)
	}
}
