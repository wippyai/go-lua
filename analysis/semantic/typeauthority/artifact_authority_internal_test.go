package typeauthority

import (
	"testing"

	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestStaticReferenceResolutionShapeLaw(t *testing.T) {
	tests := []struct {
		name       string
		resolution programstatic.TypeRefResolution
		children   int
		unresolved bool
		ok         bool
	}{
		{name: "unresolved leaf", resolution: programstatic.TypeRefUnresolved, children: 0, unresolved: true, ok: true},
		{name: "unresolved target", resolution: programstatic.TypeRefUnresolved, children: 1},
		{name: "declaration target", resolution: programstatic.TypeRefDeclaration, children: 1, ok: true},
		{name: "declaration missing", resolution: programstatic.TypeRefDeclaration, children: 0},
		{name: "declaration extra", resolution: programstatic.TypeRefDeclaration, children: 2},
		{name: "canonical target", resolution: programstatic.TypeRefCanonicalPath, children: 1, ok: true},
		{name: "canonical missing", resolution: programstatic.TypeRefCanonicalPath, children: 0},
		{name: "canonical extra", resolution: programstatic.TypeRefCanonicalPath, children: 2},
		{name: "unknown resolution", resolution: programstatic.TypeRefResolution(0), children: 1},
		{name: "future resolution", resolution: programstatic.TypeRefResolution(255), children: 1},
		{name: "negative cardinality", resolution: programstatic.TypeRefDeclaration, children: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unresolved, ok := staticReferenceResolutionShape(test.resolution, test.children)
			if unresolved != test.unresolved || ok != test.ok {
				t.Fatalf("staticReferenceResolutionShape(%d, %d) = (%v, %v), want (%v, %v)", test.resolution, test.children, unresolved, ok, test.unresolved, test.ok)
			}
		})
	}
}
