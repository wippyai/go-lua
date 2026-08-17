package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestSemanticIdentityQueriesTrackOperationAndOutcomeOwners(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("semantic-id", []OutcomeSpec{{
		Kind:   flowkind.OutcomeNormal,
		Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
	}})}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"semantic-id"}})
	if !ok {
		t.Fatal("semantic identity operation missing")
	}
	operationID, ok := contract.OperationContentID(op)
	if !ok || !operationID.Available() {
		t.Fatalf("OperationContentID = %v/%v", operationID, ok)
	}
	outcomeID, ok := contract.OutcomeContentID(op, 0)
	if !ok || !outcomeID.Available() || operationID == outcomeID {
		t.Fatalf("OutcomeContentID = %v/%v; operation identity = %v", outcomeID, ok, operationID)
	}
	again, againOK := contract.OperationContentID(op)
	if !againOK || again != operationID {
		t.Fatal("operation identity was not replay-stable")
	}
}
