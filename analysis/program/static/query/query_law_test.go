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
	draft, err := static.Build(primitiveInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	component, err := finalizer.Commit(static.CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return component
}

func TestLifecycleViewExpiresAndPublishedViewRetainsCanonicalQueries(t *testing.T) {
	draft, err := static.Build(primitiveInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View()
	if !construction.Available() || construction.Types().Primitives().Count() != 1 {
		t.Fatal("claimed construction View did not expose its authored owner rows")
	}
	if construction.StaticTypes().Count() != 0 || construction.LocalContainment().Count() != 1 {
		t.Fatal("construction View exposed an enduring type capability or lost its proof")
	}
	component, err := finalizer.Commit(static.CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Available() || construction.Types().Primitives().Count() != 0 ||
		construction.StaticTypes().Count() != 0 || construction.LocalContainment().Count() != 0 {
		t.Fatal("expired construction View retained a query projection")
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
