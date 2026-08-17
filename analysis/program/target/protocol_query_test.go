package target

import "testing"

func TestProtocolQueriesResolveCanonicalStatesAndTransitions(t *testing.T) {
	contract := mustSeal(t, deltaProtocolTransition(false))
	protocol, ok := contract.ProtocolAt(0)
	if !ok || contract.ProtocolCount() != 1 {
		t.Fatalf("protocol handle = %d/%v count=%d", protocol, ok, contract.ProtocolCount())
	}
	if got := contract.StateCount(protocol); got != 3 {
		t.Fatalf("StateCount = %d, want 3", got)
	}
	state, ok := contract.StateAt(protocol, 1)
	if !ok {
		t.Fatal("normal protocol state missing")
	}
	if final, ok := contract.StateFinal(protocol, state); !ok || !final {
		t.Fatalf("normal state finality = %v/%v, want true/true", final, ok)
	}
	if got := contract.TransitionCount(protocol); got != 1 {
		t.Fatalf("TransitionCount = %d, want 1", got)
	}
	if _, _, _, _, ok := contract.TransitionAt(protocol, 0); !ok {
		t.Fatal("protocol transition missing")
	}
}
