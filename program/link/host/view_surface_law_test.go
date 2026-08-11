package host_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program/link/host"
)

func TestPublicViewsAreNarrowAndNonEmbedded(t *testing.T) {
	component := reflect.TypeOf((*host.Component)(nil))
	wantComponent := map[string]bool{
		"ContentID": true, "Cold": true,
		"Capabilities": true, "CapabilitySeeds": true,
		"Exposures": true, "Members": true,
		"BootRoots": true, "Attachments": true, "Globals": true,
		"SemanticSourceReceipt": true, "SemanticSourceViews": true,
	}
	if component.NumMethod() != len(wantComponent) {
		t.Fatalf("Component method count = %d", component.NumMethod())
	}
	for i := 0; i < component.NumMethod(); i++ {
		if !wantComponent[component.Method(i).Name] {
			t.Fatalf("Component leaked %s", component.Method(i).Name)
		}
	}
	for _, view := range []reflect.Type{
		reflect.TypeOf(host.Capabilities{}), reflect.TypeOf(host.CapabilitySeeds{}),
		reflect.TypeOf(host.Exposures{}), reflect.TypeOf(host.Members{}),
		reflect.TypeOf(host.BootRoots{}), reflect.TypeOf(host.Attachments{}), reflect.TypeOf(host.Globals{}),
	} {
		if view.NumMethod() == 0 || view.NumMethod() > 16 {
			t.Fatalf("%s surface = %d", view, view.NumMethod())
		}
		for i := 0; i < view.NumField(); i++ {
			if view.Field(i).Anonymous {
				t.Fatalf("%s embeds %s", view, view.Field(i).Name)
			}
		}
	}
}
