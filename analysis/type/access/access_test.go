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

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
