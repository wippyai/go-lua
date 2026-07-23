package readmodel

import (
	"fmt"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRegistrationDiscriminantTraversalHasNoSemanticDepthLimit(t *testing.T) {
	discriminated := testRegistrationDiscriminatedType()
	const wrappers = 300
	deep := discriminated
	for i := 0; i < wrappers; i++ {
		deep = typetable.NewRecord().Field(fmt.Sprintf("level_%03d", i), deep).Build()
	}
	root := pathdom.NewPath(symbol.ID(1), "registry")
	domains := (Reader{}).registrationStringDiscriminantDomainsForType(
		root, nil, deep, make(map[typ.Type]struct{}),
	)
	if len(domains) != 1 {
		t.Fatalf("domains = %#v, want one domain below %d record levels", domains, wrappers)
	}
	if got := len(domains[0].target.Segments); got != wrappers+1 {
		t.Fatalf("target depth = %d, want %d", got, wrappers+1)
	}
}

func TestRegistrationDiscriminantTraversalUsesFiniteRecursiveBasis(t *testing.T) {
	discriminated := testRegistrationDiscriminatedType()
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", self).
			Field("payload", discriminated).
			Build()
	})
	root := pathdom.NewPath(symbol.ID(1), "registry")
	domains := (Reader{}).registrationStringDiscriminantDomainsForType(
		root, nil, recursive, make(map[typ.Type]struct{}),
	)
	if len(domains) != 1 {
		t.Fatalf("domains = %#v, want one finite recursive-basis domain", domains)
	}
	want := []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentField, Name: "kind"},
	}
	if got := domains[0].target.Segments; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("target = %#v, want %#v", got, want)
	}
}

func testRegistrationDiscriminatedType() typ.Type {
	left := typetable.NewRecord().Field("kind", typ.LiteralString("left")).Build()
	right := typetable.NewRecord().Field("kind", typ.LiteralString("right")).Build()
	return typ.MaterializeUnion([]typ.Type{left, right})
}
