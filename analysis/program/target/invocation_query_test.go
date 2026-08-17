package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestInvocationQueriesExposeCallbackOwnedRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{callbackOwnerOperation("invoke-query")}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"invoke-query"}})
	if !ok {
		t.Fatal("callback owner operation missing")
	}
	id, ok := contract.CallbackAt(op, 0)
	if !ok {
		t.Fatal("callback handle missing")
	}
	if owner, ok := contract.CallbackOwner(id); !ok || owner != op {
		t.Fatalf("CallbackOwner = %d/%v, want %d/true", owner, ok, op)
	}
	if function, ok := contract.CallbackFunction(id); !ok || function != (InputSource{Kind: InputSourceValueFormal, Ordinal: 0}) {
		t.Fatalf("CallbackFunction = %#v/%v", function, ok)
	}
	if _, ok := contract.CallbackOutcome(id, flowkind.OutcomeNormal); !ok {
		t.Fatal("callback normal outcome unavailable")
	}
	if _, ok := contract.CallbackAdmission(id); !ok {
		t.Fatal("callback admission unavailable")
	}
}
