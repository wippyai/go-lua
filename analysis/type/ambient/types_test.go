package ambient

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
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
