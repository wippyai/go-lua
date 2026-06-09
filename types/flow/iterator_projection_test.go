package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIteratorVarTypesIndexedArrayAndDynamicSource(t *testing.T) {
	got, ok := IteratorVarTypes(IterateIndexed, 3, typ.NewArray(typ.String))
	if !ok {
		t.Fatal("IteratorVarTypes indexed array did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Integer) || !typ.TypeEquals(got[1], typ.String) || got[2] != nil {
		t.Fatalf("indexed array vars = %#v, want integer/string/nil", got)
	}

	got, ok = IteratorVarTypes(IterateIndexed, 2, typ.NewOptional(typ.Any))
	if !ok {
		t.Fatal("IteratorVarTypes indexed optional-any did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Integer) || !typ.TypeEquals(got[1], typ.Any) {
		t.Fatalf("indexed optional-any vars = %#v, want integer/any", got)
	}

	got, ok = IteratorVarTypes(IterateIndexed, 2, typ.Any)
	if !ok {
		t.Fatal("IteratorVarTypes indexed any did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Integer) || !typ.TypeEquals(got[1], typ.Any) {
		t.Fatalf("indexed any vars = %#v, want integer/any", got)
	}
}

func TestIteratorVarTypesKeyedUniformContainers(t *testing.T) {
	mapType := typ.NewMap(typ.String, typ.Number)
	got, ok := IteratorVarTypes(IterateKeyed, 2, mapType)
	if !ok {
		t.Fatal("IteratorVarTypes keyed map did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.String) || !typ.TypeEquals(got[1], typ.Number) {
		t.Fatalf("keyed map vars = %#v, want string/number", got)
	}

	recordMap := typ.NewRecord().
		Field("fixed", typ.Boolean).
		MapComponent(typ.String, typ.Number).
		Build()
	got, ok = IteratorVarTypes(IterateKeyed, 2, recordMap)
	if !ok {
		t.Fatal("IteratorVarTypes keyed record map did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.String) || !typ.TypeEquals(got[1], typ.NewUnion(typ.Boolean, typ.Number)) {
		t.Fatalf("keyed record-map vars = %#v, want string/boolean|number", got)
	}
}

func TestIteratorVarTypesKeyedAnyAndClosedRecord(t *testing.T) {
	got, ok := IteratorVarTypes(IterateKeyed, 2, typ.Any)
	if !ok {
		t.Fatal("IteratorVarTypes keyed any did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Any) || !typ.TypeEquals(got[1], typ.Any) {
		t.Fatalf("keyed any vars = %#v, want any/any", got)
	}

	closed := typ.NewRecord().Field("name", typ.String).Build()
	got, ok = IteratorVarTypes(IterateKeyed, 2, closed)
	if !ok {
		t.Fatal("IteratorVarTypes rejected closed record with finite present entries")
	}
	if !typ.TypeEquals(got[0], typ.LiteralString("name")) || !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("closed record vars = %#v, want literal name/string", got)
	}

	if _, ok := IteratorVarTypes(IterateKeyed, 2, typ.NewMap(typ.String, typ.Nil)); ok {
		t.Fatal("IteratorVarTypes accepted keyed map with no present entries")
	}
}

func TestProjectIteratorVarTypesKeyedEmptyContainer(t *testing.T) {
	proj, ok := ProjectIteratorVarTypes(IterateKeyed, 2, typ.NewRecord().Build())
	if !ok || !proj.Empty {
		t.Fatalf("ProjectIteratorVarTypes(empty record) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectIteratorVarTypes(IterateKeyed, 2, typ.NewMap(typ.String, typ.Nil))
	if !ok || !proj.Empty {
		t.Fatalf("ProjectIteratorVarTypes(nil-valued map) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectIteratorVarTypes(IterateKeyed, 2, typ.Never)
	if !ok || !proj.Empty {
		t.Fatalf("ProjectIteratorVarTypes(never) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectIteratorVarTypes(IterateIndexed, 2, typ.Never)
	if !ok || !proj.Empty {
		t.Fatalf("ProjectIteratorVarTypes(indexed never) = %#v, %v; want recognized empty", proj, ok)
	}
}

func TestIsUniformKeyedContainer(t *testing.T) {
	if !IsUniformKeyedContainer(typ.NewMap(typ.String, typ.Number)) {
		t.Fatal("map should be uniform keyed")
	}
	if !IsUniformKeyedContainer(typ.NewRecord().Field("kind", typ.LiteralString("x")).Build()) {
		t.Fatal("closed field-only record should be a finite keyed iterator")
	}
	open := typ.NewRecord().Field("kind", typ.String).SetOpen(true).Build()
	if !IsUniformKeyedContainer(open) {
		t.Fatal("open record should be uniform keyed")
	}
	if IsUniformKeyedContainer(typ.String) || IsUniformKeyedContainer(nil) {
		t.Fatal("non-container should not be uniform keyed")
	}
}

func TestIteratorVarTypesRejectsUnknownAndInvalidKind(t *testing.T) {
	if _, ok := IteratorVarTypes(IterateIndexed, 2, typ.Unknown); ok {
		t.Fatal("unknown source should not project iterator vars")
	}
	if _, ok := IteratorVarTypes(IteratorKind(99), 2, typ.NewArray(typ.String)); ok {
		t.Fatal("invalid iterator kind should not project vars")
	}
	if _, ok := IteratorVarTypes(IterateIndexed, 0, typ.NewArray(typ.String)); ok {
		t.Fatal("zero target count should not project vars")
	}
	if got, ok := IteratorVarTypes(IterateIndexed, 2, typ.Nil); ok && got[1] != nil && got[1].Kind() != kind.Nil {
		t.Fatalf("nil source should not invent non-nil iterator element: %#v", got)
	}
}
