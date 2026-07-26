package typestate

import (
	"bytes"
	"testing"
)

func TestBuiltinLifecycleDefinitionsAndPublicationTransitions(t *testing.T) {
	if err := ValidateBuiltinDefinitions(); err != nil {
		t.Fatalf("built-in definitions: %v", err)
	}
	connection := Resource{ID: ResourceID("connection\xff"), Protocol: ProtocolConnection}
	open, ok := AcquirePublication(connection, StateOpen, Obligation{Final: StateClosed})
	if !ok || !open.Requires(StateOpen) || !open.LocallyControlled() {
		t.Fatalf("open connection = %#v/%v", open, ok)
	}
	closed, ok := open.Transition(StateClosed)
	if !ok || !closed.Requires(StateClosed) || closed.Slot.Locality != LocalityClosed {
		t.Fatalf("closed connection = %#v/%v", closed, ok)
	}
	if _, ok := closed.Transition(StateOpen); ok {
		t.Fatal("undeclared closed -> open transition was accepted")
	}
	escapedClosed, changed := closed.Escape()
	if !changed || escapedClosed.Slot.Locality != LocalityEscaped {
		t.Fatal("closed resource retained local authority after escape")
	}
	escaped, ok := open.Escape()
	if !ok || escaped.LocallyControlled() || escaped.Slot.Current != StateOpen {
		t.Fatalf("escaped connection = %#v/%v", escaped, ok)
	}
}

func TestPublicationCodecRoundTripsAndFailsClosed(t *testing.T) {
	resource := Resource{ID: ResourceID("transaction\xff"), Protocol: ProtocolTransaction}
	active, ok := AcquirePublication(resource, StateActive, Obligation{Final: StateCommitted})
	if !ok {
		t.Fatal("transaction acquisition was rejected")
	}
	encoded, ok := EncodePublication(active)
	if !ok {
		t.Fatal("transaction publication was not encoded")
	}
	decoded, ok := DecodePublication(encoded)
	if !ok || decoded != active {
		t.Fatalf("decoded publication = %#v/%v, want %#v", decoded, ok, active)
	}
	for name, malformed := range map[string][]byte{
		"raw state":      []byte(StateActive),
		"unknown state":  bytes.Replace(encoded, []byte(`"active"`), []byte(`"absent"`), 1),
		"unknown field":  append(encoded[:len(encoded)-1], []byte(`,"extra":true}`)...),
		"trailing bytes": append(append([]byte(nil), encoded...), []byte("x")...),
	} {
		if publication, accepted := DecodePublication(malformed); accepted {
			t.Errorf("%s decoded as %#v", name, publication)
		}
	}
}
