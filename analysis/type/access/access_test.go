package access

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestFieldDirectRecordField(t *testing.T) {
	rec := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, ok := Field(rec, "name")
	if !ok {
		t.Fatal("Field(record, name) failed")
	}
	assertType(t, got, typ.String)

	if _, ok := Field(rec, "missing"); ok {
		t.Fatal("Field(record, missing) succeeded")
	}
}

func TestFieldInterfaceMethodSubstitutesSelf(t *testing.T) {
	iface := typ.NewInterface("Reader", []typ.Method{
		{
			Name: "read",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Self).
				Build(),
		},
	})

	got, ok := Field(iface, "read")
	if !ok {
		t.Fatal("Field(interface method, read) failed")
	}
	want := typ.Func().Param("self", iface).Returns(iface).Build()
	assertType(t, got, want)

	if _, ok := Field(iface, "missing"); ok {
		t.Fatal("Field(interface method, missing) succeeded")
	}
}

func TestBuiltinTableTopMarkerAccess(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)

	got, ok := Field(tableTop, "dynamic")
	if !ok {
		t.Fatal("Field(table top marker, dynamic) failed")
	}
	assertType(t, got, typ.Any)

	got, ok = Index(tableTop, typ.LiteralString("dynamic"))
	if !ok {
		t.Fatal("Index(table top marker, dynamic) failed")
	}
	assertType(t, got, typ.Any)
}

func TestFieldOptionalAliasInstantiatedRecord(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.String).Build()

		got, ok := Field(typeexpr.Optional(rec), "value")
		if !ok {
			t.Fatal("Field(optional record, value) failed")
		}
		assertType(t, got, typeexpr.Optional(typ.String))
	})

	t.Run("optional literal", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.LiteralString("ok")).Build()

		got, ok := Field(typeexpr.Optional(rec), "value")
		if !ok {
			t.Fatal("Field(optional literal record, value) failed")
		}
		assertType(t, got, typeexpr.Optional(typ.LiteralString("ok")))
	})

	t.Run("alias", func(t *testing.T) {
		rec := typetable.NewRecord().Field("value", typ.Boolean).Build()

		got, ok := Field(typ.NewAlias("Row", rec), "value")
		if !ok {
			t.Fatal("Field(alias record, value) failed")
		}
		assertType(t, got, typ.Boolean)
	})

	t.Run("instantiated", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{param},
			typetable.NewRecord().Field("value", param).Build())

		got, ok := Field(typ.Instantiate(box, typ.Number), "value")
		if !ok {
			t.Fatal("Field(Box<number>, value) failed")
		}
		assertType(t, got, typ.Number)
	})

	t.Run("constrained type parameter", func(t *testing.T) {
		constraint := typetable.NewRecord().Field("value", typ.String).Build()
		param := typ.NewTypeParam("T", constraint)

		got, ok := Field(param, "value")
		if !ok {
			t.Fatal("Field(constrained T, value) failed")
		}
		assertType(t, got, typ.String)
	})

	t.Run("unconstrained type parameter", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)

		if got, ok := Field(param, "value"); ok {
			t.Fatalf("Field(unconstrained T, value) = %v/true, want missing", got)
		}
	})
}

func TestFieldWrappedTopTypesPreserveAccessTopSemantics(t *testing.T) {
	got, ok := Field(typ.NewAlias("Dynamic", typ.Any), "value")
	if !ok {
		t.Fatal("Field(alias any, value) failed")
	}
	assertType(t, got, typ.Any)

	got, ok = Field(typ.NewAlias("Opaque", typ.Unknown), "value")
	if !ok {
		t.Fatal("Field(alias unknown, value) failed")
	}
	assertType(t, got, typ.Unknown)
}

func TestFieldMapStringFieldOptionalityAndMissingPolicy(t *testing.T) {
	m := typetable.NewMap(typ.String, typ.Number)

	got, ok := Field(m, "dynamic")
	if !ok {
		t.Fatal("Field(map, dynamic) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))

	if !MissingFieldReadsNil(m) {
		t.Fatal("MissingFieldReadsNil(map) = false, want true")
	}

	rec := typetable.NewRecord().
		MapComponent(typ.String, typ.Boolean).
		Build()
	got, ok = Field(rec, "dynamic")
	if !ok {
		t.Fatal("Field(record map component, dynamic) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))
}

func TestFieldMapComponentOptionalizesAnnotatedUnionEvidence(t *testing.T) {
	ann := []annotation.Annotation{{Name: "tag"}}
	rec := typetable.NewRecord().
		MapComponent(typ.String, typ.NewAnnotated(typeexpr.Union(typ.String, typ.Number), ann)).
		Build()

	got, ok := Field(rec, "dynamic")
	if !ok {
		t.Fatal("Field(record annotated union map component, dynamic) failed")
	}
	assertType(t, got, typeexpr.Union(typ.Nil, typ.String, typ.Number))
}

func TestFieldRecordMapComponentKeepsOptionalMapValueShape(t *testing.T) {
	rec := typetable.NewRecord().
		MapComponent(typ.String, typeexpr.Optional(typ.Number)).
		Build()

	got, ok := Field(rec, "dynamic")
	if !ok {
		t.Fatal("Field(record optional map component, dynamic) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Number))
}

func TestFieldOpenRecordReturnsUnknown(t *testing.T) {
	rec := typetable.NewRecord().
		SetOpen(true).
		Build()

	got, ok := Field(rec, "dynamic")
	if !ok {
		t.Fatal("Field(open record, dynamic) failed")
	}
	assertType(t, got, typ.Unknown)
}

func TestFieldRecordMapComponentUsesStrictFieldAdmission(t *testing.T) {
	m := typetable.NewMap(typ.LiteralString("status"), typ.Boolean)

	got, ok := Field(m, "status")
	if !ok {
		t.Fatal("Field(literal-key map, status) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	if _, ok := Field(m, "other"); ok {
		t.Fatal("Field(literal-key map, other) succeeded")
	}

	rec := typetable.NewRecord().
		MapComponent(typ.LiteralString("status"), typ.Boolean).
		Build()

	got, ok = Field(rec, "status")
	if !ok {
		t.Fatal("Field(record literal map component, status) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.Boolean))

	if _, ok := Field(rec, "other"); ok {
		t.Fatal("Field(record literal map component, other) succeeded")
	}
}

func TestRecordStringFieldAndIndexSharePrecedence(t *testing.T) {
	rec := typetable.NewRecord().
		Field("field", typ.String).
		StaticStringIndex("static", typ.Number).
		MapComponent(typ.String, typ.Boolean).
		Build()

	for _, tc := range []struct {
		name string
		key  string
		want typ.Type
	}{
		{name: "declared field", key: "field", want: typ.String},
		{name: "static string member", key: "static", want: typ.Number},
		{name: "map component", key: "dynamic", want: typeexpr.Optional(typ.Boolean)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fieldGot, fieldOK := Field(rec, tc.key)
			if !fieldOK {
				t.Fatalf("Field(record, %s) failed", tc.key)
			}
			assertType(t, fieldGot, tc.want)

			indexGot, indexOK := Index(rec, typ.LiteralString(tc.key))
			if !indexOK {
				t.Fatalf("Index(record, %s) failed", tc.key)
			}
			assertType(t, indexGot, tc.want)
		})
	}

	open := typetable.NewRecord().SetOpen(true).Build()
	fieldGot, fieldOK := Field(open, "missing")
	if !fieldOK {
		t.Fatal("Field(open record, missing) failed")
	}
	assertType(t, fieldGot, typ.Unknown)
	indexGot, indexOK := Index(open, typ.LiteralString("missing"))
	if !indexOK {
		t.Fatal("Index(open record, missing) failed")
	}
	assertType(t, indexGot, typ.Unknown)
}

func TestFieldCommonUnionField(t *testing.T) {
	left := typetable.NewRecord().
		Field("id", typ.String).
		Field("left", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("id", typ.String).
		Field("right", typ.Boolean).
		Build()

	got, ok := Field(typeexpr.Union(left, right), "id")
	if !ok {
		t.Fatal("Field(union, id) failed")
	}
	assertType(t, got, typ.String)
}

func TestFieldUnionMissingMemberContributesNil(t *testing.T) {
	event := typetable.NewRecord().Field("kind", typ.String).Build()
	timer := typetable.NewRecord().Field("elapsed", typ.Number).Build()

	got, ok := Field(typeexpr.Union(event, timer), "kind")
	if !ok {
		t.Fatal("Field(union missing member, kind) failed")
	}
	assertType(t, got, typeexpr.Optional(typ.String))
}

func TestFieldIntersectionFieldMeetPolicy(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		right typ.Type
		want  typ.Type
	}{
		{
			name:  "any and string",
			left:  typ.Any,
			right: typ.String,
			want:  typ.String,
		},
		{
			name:  "never and string",
			left:  typ.Never,
			right: typ.String,
			want:  typ.Never,
		},
		{
			name:  "nil and optional string",
			left:  typ.Nil,
			right: typeexpr.Optional(typ.String),
			want:  typ.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := typetable.NewRecord().Field("value", tt.left).Build()
			right := typetable.NewRecord().Field("value", tt.right).Build()

			got, ok := Field(typeexpr.Intersection(left, right), "value")
			if !ok {
				t.Fatal("Field(intersection, value) failed")
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestFieldIntersectionAllowsDisjointRecordAndInterfaceMembers(t *testing.T) {
	fields := typetable.NewRecord().
		Field("platform", typ.String).
		Build()
	methods := typ.NewInterface("os", []typ.Method{
		{
			Name: "time",
			Type: typ.Func().Returns(typ.Number).Build(),
		},
	})

	got, ok := Field(typeexpr.Intersection(fields, methods), "time")
	if !ok {
		t.Fatal("Field(record & interface, time) failed")
	}
	assertType(t, got, typ.Func().Returns(typ.Number).Build())

	got, ok = Field(typeexpr.Intersection(fields, methods), "platform")
	if !ok {
		t.Fatal("Field(record & interface, platform) failed")
	}
	assertType(t, got, typ.String)
}

func TestFieldTerminatesOnMutualRecursiveWrapperCycle(t *testing.T) {
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(right)
	right.SetBody(left)
	if got, ok := Field(left, "missing"); ok || got != nil {
		t.Fatalf("Field(mutual cycle) = (%v, %v), want no field", got, ok)
	}
}

func TestFieldRecursiveUnionUsesExactMustFixedPoint(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	record := typetable.NewRecord().Field("value", typ.String).Build()
	node.SetBody(typeexpr.Union(node, record))

	got, ok := Field(node, "value")
	if !ok {
		t.Fatal("Field(recursive union, value) failed")
	}
	assertType(t, got, typ.String)
	if !MissingFieldReadsNil(node) {
		t.Fatal("recursive union of table arms lost missing-read nil semantics")
	}

	bad := typ.NewRecursivePlaceholder("Bad")
	bad.SetBody(typeexpr.Union(bad, typ.Boolean))
	if got, ok := Field(bad, "value"); ok || got != nil {
		t.Fatalf("Field(recursive union with non-table arm) = (%v, %v)", got, ok)
	}
}

func TestFieldDeepCompositeProjectionUsesExplicitWorkStack(t *testing.T) {
	const depth = 4097
	leaf := typetable.NewRecord().Field("value", typ.String).Build()
	var value typ.Type = leaf
	for index := 0; index < depth; index++ {
		value = &typ.Union{Members: []typ.Type{value, leaf}}
	}
	got, ok := Field(value, "value")
	if !ok {
		t.Fatal("deep composite field projection failed")
	}
	assertType(t, got, typ.String)
}

func TestShallowAccessTraversalDoesNotAllocate(t *testing.T) {
	record := typetable.NewRecord().Field("value", typ.String).Build()
	array := typ.NewArray(typ.String)
	key := typ.Integer
	if got := testing.AllocsPerRun(1000, func() {
		_, _ = Field(record, "value")
	}); got != 0 {
		t.Fatalf("Field shallow allocations = %v, want 0", got)
	}
	if got := testing.AllocsPerRun(1000, func() {
		_, _ = Index(array, key)
	}); got > 1 {
		t.Fatalf("Index shallow allocations = %v, want at most the existing projector closure", got)
	}
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
