package ambient

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestLookupChannelGeneric(t *testing.T) {
	got, ok := Lookup(Channel)
	if !ok {
		t.Fatal("Lookup(Channel) returned ok=false")
	}
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
	got, ok := Lookup(Table)
	if !ok {
		t.Fatal("Lookup(Table) returned ok=false")
	}
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
