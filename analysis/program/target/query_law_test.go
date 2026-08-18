package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"
)

func TestOperationQueriesKeepBoundPrefixAndOpaqueLast(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		builtin("query-a", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
		builtin("query-b", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed}),
	}})
	if got := contract.OperationCount(); got != 3 {
		t.Fatalf("OperationCount = %d, want bound operations plus opaque", got)
	}
	if got := contract.boundOperationCount(); got != 2 {
		t.Fatalf("BoundOperationCount = %d, want 2", got)
	}
	op, ok := contract.OperationAt(contract.OperationCount() - 1)
	if !ok {
		t.Fatal("opaque operation missing")
	}
	if opaque, ok := contract.Opaque(); !ok || opaque != op {
		t.Fatalf("Opaque = %d/%v, want %d/true", opaque, ok, op)
	}
	if _, ok := contract.OperationAt(contract.OperationCount()); ok {
		t.Fatal("out-of-range operation resolved")
	}
}

func TestInvocationQueriesExposeCallbackOwnedRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{callbackOwnerOperation("invoke-query")}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"invoke-query"}})
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
	if function, ok := contract.callbackFunction(id); !ok || function != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) {
		t.Fatalf("CallbackFunction = %#v/%v", function, ok)
	}
	if _, ok := contract.CallbackOutcome(id, flowkind.OutcomeNormal); !ok {
		t.Fatal("callback normal outcome unavailable")
	}
	if _, ok := contract.callbackAdmission(id); !ok {
		t.Fatal("callback admission unavailable")
	}
}

func TestContinuationQueriesReturnCanonicalSuspensionCoordinates(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{deltaSuspension(vocabulary.ReentryMany)}})
	op, ok := contract.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"suspend"}})
	if !ok || contract.suspensionCount(op) != 1 {
		t.Fatalf("suspension lookup = %d/%v", contract.suspensionCount(op), ok)
	}
	yield, reentry, source, multiplicity, ok := contract.suspensionAt(op, 0)
	if !ok || yield != 1 || reentry != 0 || source != vocabulary.ReentryByCall || multiplicity != vocabulary.ReentryMany {
		t.Fatalf("SuspensionAt = %d/%d/%d/%d/%v", yield, reentry, source, multiplicity, ok)
	}
	if _, _, _, _, ok := contract.suspensionAt(op, 1); ok {
		t.Fatal("out-of-range suspension query resolved")
	}
}

func TestProtocolQueriesResolveCanonicalStatesAndTransitions(t *testing.T) {
	contract := mustSeal(t, deltaProtocolTransition(false))
	protocol, ok := contract.protocolAt(0)
	if !ok || contract.protocolCount() != 1 {
		t.Fatalf("protocol handle = %d/%v count=%d", protocol, ok, contract.protocolCount())
	}
	if got := contract.stateCount(protocol); got != 3 {
		t.Fatalf("StateCount = %d, want 3", got)
	}
	state, ok := contract.stateAt(protocol, 1)
	if !ok {
		t.Fatal("normal protocol state missing")
	}
	if final, ok := contract.stateFinal(protocol, state); !ok || !final {
		t.Fatalf("normal state finality = %v/%v, want true/true", final, ok)
	}
	if got := contract.transitionCount(protocol); got != 1 {
		t.Fatalf("TransitionCount = %d, want 1", got)
	}
	if _, _, _, _, ok := contract.transitionAt(protocol, 0); !ok {
		t.Fatal("protocol transition missing")
	}
}
