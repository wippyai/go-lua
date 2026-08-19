package typeauthority

import (
	"testing"

	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
)

func TestStaticReferenceResolutionShapeLaw(t *testing.T) {
	tests := []struct {
		name       string
		resolution staticrefs.Resolution
		children   int
		unresolved bool
		ok         bool
	}{
		{name: "unresolved leaf", resolution: staticrefs.Unresolved, children: 0, unresolved: true, ok: true},
		{name: "unresolved target", resolution: staticrefs.Unresolved, children: 1},
		{name: "declaration target", resolution: staticrefs.Declaration, children: 1, ok: true},
		{name: "declaration missing", resolution: staticrefs.Declaration, children: 0},
		{name: "declaration extra", resolution: staticrefs.Declaration, children: 2},
		{name: "canonical target", resolution: staticrefs.CanonicalPath, children: 1, ok: true},
		{name: "canonical missing", resolution: staticrefs.CanonicalPath, children: 0},
		{name: "canonical extra", resolution: staticrefs.CanonicalPath, children: 2},
		{name: "unknown resolution", resolution: staticrefs.Resolution(0), children: 1},
		{name: "future resolution", resolution: staticrefs.Resolution(255), children: 1},
		{name: "negative cardinality", resolution: staticrefs.Declaration, children: -1},
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
