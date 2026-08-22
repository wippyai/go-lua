package typestate

import "testing"

func TestProtocolFromStringRejectsEmptyAndPreservesNames(t *testing.T) {
	if got, ok := ProtocolFromString(""); ok || got != "" {
		t.Fatalf("ProtocolFromString(empty) = %q, %v; want empty, false", got, ok)
	}

	got, ok := ProtocolFromString("made-up-protocol")
	if !ok {
		t.Fatal("ProtocolFromString(non-empty) rejected user-defined protocol")
	}
	if got.String() != "made-up-protocol" {
		t.Fatalf("protocol String() = %q", got.String())
	}
}

func TestStateFromStringRejectsEmptyAndPreservesNames(t *testing.T) {
	if got, ok := StateFromString(""); ok || got != "" {
		t.Fatalf("StateFromString(empty) = %q, %v; want empty, false", got, ok)
	}

	got, ok := StateFromString("half_open")
	if !ok {
		t.Fatal("StateFromString(non-empty) rejected user-defined state")
	}
	if got.String() != "half_open" {
		t.Fatalf("state String() = %q", got.String())
	}
}
