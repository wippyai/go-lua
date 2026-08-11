package module_test

import (
	"reflect"
	"testing"

	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// This is intentionally in module_test: promoted methods would be visible to
// consumers even when they are not visible to package-internal inspection.
func TestPublishedViewsAreNarrow(t *testing.T) {
	allowedComponent := map[string]bool{
		"Actors": true, "Roots": true, "Cache": true, "Coordinates": true,
		"Generations": true, "Outcomes": true, "ReadySubjects": true,
		"Terminals": true, "Cold": true, "ContentID": true,
		"MatchesProject": true, "MatchesBoundary": true,
		"HostRelationID": true,
	}
	component := reflect.TypeOf(&linkmodule.Component{})
	for index := 0; index < component.NumMethod(); index++ {
		name := component.Method(index).Name
		if !allowedComponent[name] {
			t.Fatalf("Component leaked %s", name)
		}
	}
	for _, view := range []reflect.Type{
		reflect.TypeOf(linkmodule.Actors{}), reflect.TypeOf(linkmodule.Roots{}),
		reflect.TypeOf(linkmodule.Cache{}), reflect.TypeOf(linkmodule.Coordinates{}),
		reflect.TypeOf(linkmodule.Generations{}), reflect.TypeOf(linkmodule.Outcomes{}),
		reflect.TypeOf(linkmodule.ReadySubjects{}), reflect.TypeOf(linkmodule.Terminals{}),
	} {
		if view.NumMethod() > 16 {
			t.Fatalf("%s exposes %d methods", view, view.NumMethod())
		}
	}
}
