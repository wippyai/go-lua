package query_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	static "github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func primitiveInput() static.Input {
	return static.Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyTypePrimitive: 1},
		Types:  statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}},
	}
}

func primitiveComponent(t *testing.T) *static.Component {
	t.Helper()
	component, _, err := static.Build(primitiveInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return component
}

func TestBuildViewAndPublishedViewRetainCanonicalQueries(t *testing.T) {
	component, construction, err := static.Build(primitiveInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !construction.Available() || construction.Types().Primitives().Count() != 1 {
		t.Fatal("Build view did not expose its authored owner rows")
	}
	if construction.StaticTypes().Count() != 1 || construction.LocalContainment().Count() != 1 {
		t.Fatal("Build view omitted its authored type capability or local proof")
	}
	published := component.View()
	if !published.Available() || published.Types().Primitives().Count() != 1 ||
		published.StaticTypes().Count() != 1 || published.LocalContainment().Count() != 0 {
		t.Fatal("published View did not expose the expected immutable query surface")
	}
}

func TestUnavailableViewFailsClosedAcrossOwnedProjections(t *testing.T) {
	var view staticquery.View
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	ref, refOK := view.StaticTypes().Ref(term)
	if view.Available() || view.ContentID().Available() || view.StaticTypes().Count() != 0 ||
		refOK || ref.Term() != 0 {
		t.Fatal("zero query View exposed authored state")
	}
	if _, ok := view.References().CanonicalAt(keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), 0); ok {
		t.Fatal("zero query View admitted a reference row")
	}
}

func TestStaticTypesRebindRawTermsWithoutRootOwnership(t *testing.T) {
	first := primitiveComponent(t)
	second := primitiveComponent(t)
	foreign, ok := first.View().StaticTypes().At(0)
	if !ok || foreign.Term() != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatal("first StaticTypes.At failed")
	}
	bound, ok := second.View().StaticTypes().Ref(foreign.Term())
	if !ok || bound.Term() != foreign.Term() {
		t.Fatal("raw static term did not rebind to the receiving snapshot")
	}
}
