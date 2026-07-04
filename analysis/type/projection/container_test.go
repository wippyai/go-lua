package projection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestElementOfContainerShapes(t *testing.T) {
	cases := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{name: "array", in: typ.NewArray(typ.String), want: typ.String},
		{name: "map", in: typetable.NewMap(typ.String, typ.Number), want: typ.Number},
		{name: "readonly map", in: typetable.NewReadonlyMap(typ.String, typ.Boolean), want: typ.Boolean},
		{name: "record map component", in: typetable.NewRecord().MapComponent(typ.Integer, typ.String).Build(), want: typ.String},
		{name: "single tuple", in: typ.NewTuple(typ.Integer), want: typ.Integer},
		{name: "multi tuple", in: typ.NewTuple(typ.String, typ.Number), want: typ.MaterializeUnion([]typ.Type{typ.String, typ.Number})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ElementOf(tc.in)
			if !ok {
				t.Fatal("ElementOf failed")
			}
			assertProjectionType(t, got, tc.want)
		})
	}
}

func TestElementOfUnwrapsOptionalAliasAndAnnotated(t *testing.T) {
	source := typ.NewAnnotated(
		typ.NewAlias("MaybeItems", typeexpr.Optional(typ.NewArray(typ.String))),
		[]annotation.Annotation{{Name: "source"}},
	)

	got, ok := ElementOf(source)
	if !ok {
		t.Fatal("ElementOf failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestElementOfUnionProjectsMembersAndSkipsNil(t *testing.T) {
	source := typeexpr.Union(
		typ.NewArray(typ.String),
		typetable.NewReadonlyMap(typ.String, typ.Number),
		typ.Nil,
	)

	got, ok := ElementOf(source)
	if !ok {
		t.Fatal("ElementOf failed")
	}
	assertProjectionType(t, got, typ.MaterializeUnion([]typ.Type{typ.String, typ.Number}))
}

func TestElementOfRejectsNonContainerUnionMember(t *testing.T) {
	if got, ok := ElementOf(typeexpr.Union(typ.NewArray(typ.String), typ.Boolean)); ok || got != nil {
		t.Fatalf("ElementOf succeeded: %v", got)
	}
}

func TestElementOfRejectsEmptyTuple(t *testing.T) {
	if got, ok := ElementOf(typ.NewTuple()); ok || got != nil {
		t.Fatalf("ElementOf succeeded: %v", got)
	}
}

func TestElementOfRejectsPastRecursionDepth(t *testing.T) {
	var source typ.Type = typ.NewArray(typ.String)
	for i := 0; i <= typ.DefaultRecursionDepth; i++ {
		source = typ.NewAnnotated(source, []annotation.Annotation{{Name: "depth"}})
	}

	if got, ok := ElementOf(source); ok || got != nil {
		t.Fatalf("ElementOf succeeded: %v", got)
	}
}

func TestElementOfRecursiveContainer(t *testing.T) {
	source := typ.NewRecursive("Items", func(typ.Type) typ.Type {
		return typ.NewArray(typ.String)
	})

	got, ok := ElementOf(source)
	if !ok {
		t.Fatal("ElementOf recursive array failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestElementOfInstantiatedContainer(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, typ.NewArray(param))

	got, ok := ElementOf(typ.Instantiate(box, typ.Number))
	if !ok {
		t.Fatal("ElementOf instantiated array failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestKeyOfContainerShapes(t *testing.T) {
	cases := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{name: "array", in: typ.NewArray(typ.String), want: typ.Integer},
		{name: "map", in: typetable.NewMap(typ.String, typ.Number), want: typ.String},
		{name: "readonly map", in: typetable.NewReadonlyMap(typ.Integer, typ.Boolean), want: typ.Integer},
		{name: "record map component", in: typetable.NewRecord().MapComponent(typ.String, typ.Boolean).Build(), want: typ.String},
		{name: "closed record fields", in: typetable.NewRecord().Field("count", typ.Number).Field("name", typ.String).Build(), want: typ.MaterializeUnion([]typ.Type{typ.LiteralString("count"), typ.LiteralString("name")})},
		{name: "tuple", in: typ.NewTuple(typ.String, typ.Number), want: typ.Integer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := KeyOf(tc.in)
			if !ok {
				t.Fatal("KeyOf failed")
			}
			assertProjectionType(t, got, tc.want)
		})
	}
}

func TestKeyOfRecursiveAndInstantiatedContainers(t *testing.T) {
	recursiveMap := typ.NewRecursive("StringMap", func(typ.Type) typ.Type {
		return typetable.NewMap(typ.String, typ.Boolean)
	})
	got, ok := KeyOf(recursiveMap)
	if !ok {
		t.Fatal("KeyOf recursive map failed")
	}
	assertProjectionType(t, got, typ.String)

	param := typ.NewTypeParam("K", nil)
	mapBox := typ.NewGeneric("MapBox", []*typ.TypeParam{param}, typetable.NewMap(param, typ.Number))
	got, ok = KeyOf(typ.Instantiate(mapBox, typ.Integer))
	if !ok {
		t.Fatal("KeyOf instantiated map failed")
	}
	assertProjectionType(t, got, typ.Integer)
}

func TestKeyOfUnionProjectsMembersAndSkipsNil(t *testing.T) {
	source := typeexpr.Union(
		typ.NewArray(typ.String),
		typetable.NewReadonlyMap(typ.String, typ.Number),
		typ.Nil,
	)

	got, ok := KeyOf(source)
	if !ok {
		t.Fatal("KeyOf failed")
	}
	assertProjectionType(t, got, typ.MaterializeUnion([]typ.Type{typ.Integer, typ.String}))
}

func TestKeyOfRejectsNonContainerUnionMember(t *testing.T) {
	if got, ok := KeyOf(typeexpr.Union(typ.NewArray(typ.String), typ.Boolean)); ok || got != nil {
		t.Fatalf("KeyOf succeeded: %v", got)
	}
}

func assertProjectionType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
