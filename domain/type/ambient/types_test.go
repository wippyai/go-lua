package ambient

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestLookupChannelGeneric(t *testing.T) {
	declaration, ok := Lookup(Channel)
	if !ok {
		t.Fatal("Lookup(Channel) returned ok=false")
	}
	got := declaration.Type()
	generic, ok := got.(*typ.Generic)
	if !ok {
		t.Fatalf("Lookup(Channel) = %T, want *typ.Generic", got)
	}
	if generic.Name != Channel || len(generic.TypeParams) != 1 {
		t.Fatalf("channel generic = %#v", generic)
	}
	if iface, ok := generic.Body.(*typ.Interface); !ok || iface.Name != Channel {
		t.Fatalf("channel body = %T/%v, want Channel interface", generic.Body, generic.Body)
	}
}

func TestLookupTableTopMarker(t *testing.T) {
	declaration, ok := Lookup(Table)
	if !ok {
		t.Fatal("Lookup(Table) returned ok=false")
	}
	got := declaration.Type()
	if !typ.IsBuiltinTableTopMarker(got) {
		t.Fatalf("Lookup(Table) = %s, want builtin table top marker", got)
	}
}

func TestChannelPayloadType(t *testing.T) {
	payload := typ.String
	channel := typ.Instantiate(ChannelGeneric(), payload)

	got, ok := ChannelPayloadType(channel)
	if !ok || !typ.TypeEquals(got, payload) {
		t.Fatalf("ChannelPayloadType(Channel<string>) = %v/%v, want %v", got, ok, payload)
	}

	if got, ok := ChannelPayloadType(typ.Number); ok || got != nil {
		t.Fatalf("ChannelPayloadType(number) = %v/%v, want nil/false", got, ok)
	}
}

func TestChannelPayloadTypeAcceptsRuntimeModuleChannel(t *testing.T) {
	payload := typ.String
	param := typ.NewTypeParam("T", nil)
	runtimeChannel := typ.NewGeneric("channel.Channel", []*typ.TypeParam{param}, typ.NewInterface("channel.Channel", nil))

	got, ok := ChannelPayloadType(typ.Instantiate(runtimeChannel, payload))
	if !ok || !typ.TypeEquals(got, payload) {
		t.Fatalf("ChannelPayloadType(channel.Channel<string>) = %v/%v, want %v", got, ok, payload)
	}

	otherChannel := typ.NewGeneric("other.Channel", []*typ.TypeParam{param}, typ.NewInterface("other.Channel", nil))
	if got, ok := ChannelPayloadType(typ.Instantiate(otherChannel, payload)); ok || got != nil {
		t.Fatalf("ChannelPayloadType(other.Channel<string>) = %v/%v, want nil/false", got, ok)
	}
}

// TestDeclarationsAreTheWholeAmbientNamespace holds the catalogue as the single
// authority: every declaration is reachable by name, and the runtime
// constructors are projections of the same rows rather than second spellings.
func TestDeclarationsAreTheWholeAmbientNamespace(t *testing.T) {
	catalogue := Declarations()
	if len(catalogue) == 0 {
		t.Fatal("ambient catalogue is empty")
	}
	names := make(map[string]bool, len(catalogue))
	for _, declaration := range catalogue {
		if declaration.Name == "" {
			t.Fatalf("ambient declaration %#v has no name", declaration)
		}
		if names[declaration.Name] {
			t.Fatalf("ambient declaration %q is declared twice", declaration.Name)
		}
		names[declaration.Name] = true
		found, ok := Lookup(declaration.Name)
		if !ok || found.Name != declaration.Name || len(found.Params) != len(declaration.Params) {
			t.Fatalf("Lookup(%q) = %#v/%v, want the catalogue row", declaration.Name, found, ok)
		}
		if declaration.Type() == nil {
			t.Fatalf("ambient declaration %q materializes no type", declaration.Name)
		}
	}
	if !names[Channel] || !names[Table] {
		t.Fatalf("ambient catalogue names = %v, want Channel and %s", names, Table)
	}
	if generic := ChannelGeneric(); generic == nil || generic.Name != Channel {
		t.Fatalf("ChannelGeneric() = %v, want the catalogue Channel row", generic)
	}
}

// TestDeclarationsIsACopy keeps the catalogue immutable from the outside: an
// enumeration a caller mutates must not become the namespace.
func TestDeclarationsIsACopy(t *testing.T) {
	first := Declarations()
	first[0].Name = "mutated"
	if second := Declarations(); second[0].Name == "mutated" {
		t.Fatal("Declarations() handed out the catalogue itself")
	}
}
