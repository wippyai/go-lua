package program

import "testing"

func TestCallIdentityQueriesKeepMissingRowsFailClosed(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-call-identity-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got, ok := published.CallIDAt(-1); ok || got.Available() {
		t.Fatalf("CallIDAt(-1) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallIDAt(0); ok || got.Available() {
		t.Fatalf("CallIDAt(0) = %x/%v for an empty call column; want unavailable", got, ok)
	}
	if got, ok := published.CallCalleeIDAt(0); ok || got.Available() {
		t.Fatalf("CallCalleeIDAt(0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallReceiverIDAt(0); ok || got.Available() {
		t.Fatalf("CallReceiverIDAt(0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallActualsIDAt(0); ok || got.Available() {
		t.Fatalf("CallActualsIDAt(0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallValuesIDAt(0); ok || got.Available() {
		t.Fatalf("CallValuesIDAt(0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallArgumentIDAt(0, 0); ok || got.Available() {
		t.Fatalf("CallArgumentIDAt(0,0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallTypeArgumentsIDAt(0); ok || got.Available() {
		t.Fatalf("CallTypeArgumentsIDAt(0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallTypeArgumentIDAt(0, 0); ok || got.Available() {
		t.Fatalf("CallTypeArgumentIDAt(0,0) = %x/%v; want unavailable", got, ok)
	}
	if got, ok := published.CallFormalIDAt(0); ok || got.Available() {
		t.Fatalf("CallFormalIDAt(0) = %x/%v; want unavailable", got, ok)
	}
}
