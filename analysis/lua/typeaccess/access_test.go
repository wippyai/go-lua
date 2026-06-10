package typeaccess

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFieldDirectRecordField(t *testing.T) {
	rec := typ.NewRecord().
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

func TestFieldOptionalAliasInstantiatedRecord(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		rec := typ.NewRecord().Field("value", typ.String).Build()

		got, ok := Field(typ.NewOptional(rec), "value")
		if !ok {
			t.Fatal("Field(optional record, value) failed")
		}
		assertType(t, got, typ.NewOptional(typ.String))
	})

	t.Run("alias", func(t *testing.T) {
		rec := typ.NewRecord().Field("value", typ.Boolean).Build()

		got, ok := Field(typ.NewAlias("Row", rec), "value")
		if !ok {
			t.Fatal("Field(alias record, value) failed")
		}
		assertType(t, got, typ.Boolean)
	})

	t.Run("instantiated", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		box := typ.NewGeneric("Box", []*typ.TypeParam{param},
			typ.NewRecord().Field("value", param).Build())

		got, ok := Field(typ.Instantiate(box, typ.Number), "value")
		if !ok {
			t.Fatal("Field(Box<number>, value) failed")
		}
		assertType(t, got, typ.Number)
	})
}

func TestFieldMapStringFieldOptionalityAndMissingPolicy(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)

	got, ok := Field(m, "dynamic")
	if !ok {
		t.Fatal("Field(map, dynamic) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Number))

	if !MissingFieldReadsNil(m) {
		t.Fatal("MissingFieldReadsNil(map) = false, want true")
	}

	rec := typ.NewRecord().
		MapComponent(typ.String, typ.Boolean).
		Build()
	got, ok = Field(rec, "dynamic")
	if !ok {
		t.Fatal("Field(record map component, dynamic) failed")
	}
	assertType(t, got, typ.NewOptional(typ.Boolean))
}

func TestFieldCommonUnionField(t *testing.T) {
	left := typ.NewRecord().
		Field("id", typ.String).
		Field("left", typ.Number).
		Build()
	right := typ.NewRecord().
		Field("id", typ.String).
		Field("right", typ.Boolean).
		Build()

	got, ok := Field(typ.NewUnion(left, right), "id")
	if !ok {
		t.Fatal("Field(union, id) failed")
	}
	assertType(t, got, typ.String)
}

func TestFieldUnionProjectionPolicy(t *testing.T) {
	fieldRec := func(t typ.Type) typ.Type {
		return typ.NewRecord().Field("value", t).Build()
	}
	optFieldRec := func(t typ.Type) typ.Type {
		return typ.NewRecord().OptField("value", t).Build()
	}

	tests := []struct {
		name     string
		receiver typ.Type
		want     typ.Type
	}{
		{
			name:     "unknown refines to concrete projection",
			receiver: typ.NewUnion(fieldRec(typ.Unknown), fieldRec(typ.String)),
			want:     typ.String,
		},
		{
			name:     "any absorbs concrete projection",
			receiver: typ.NewUnion(fieldRec(typ.Any), fieldRec(typ.String)),
			want:     typ.Any,
		},
		{
			name:     "never is ignored as impossible projection",
			receiver: typ.NewUnion(fieldRec(typ.Never), fieldRec(typ.String)),
			want:     typ.String,
		},
		{
			name:     "optional field preserves nilability",
			receiver: typ.NewUnion(fieldRec(typ.String), optFieldRec(typ.Number)),
			want:     typ.NewUnion(typ.Nil, typ.String, typ.Number),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Field(tt.receiver, "value")
			if !ok {
				t.Fatal("Field(union, value) failed")
			}
			assertType(t, got, tt.want)
		})
	}
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
			right: typ.NewOptional(typ.String),
			want:  typ.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := typ.NewRecord().Field("value", tt.left).Build()
			right := typ.NewRecord().Field("value", tt.right).Build()

			got, ok := Field(typ.NewIntersection(left, right), "value")
			if !ok {
				t.Fatal("Field(intersection, value) failed")
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestCallableReturnFirstReturn(t *testing.T) {
	fn := typ.Func().
		Param("input", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := CallableReturn(fn)
	if !ok {
		t.Fatal("CallableReturn(function) failed")
	}
	assertType(t, got, typ.Number)
}

func TestCallableReturnUnionProjectionUsesNormalizePackage(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	returns := []typ.Type{typ.Unknown, typ.String, typ.Never}
	got, ok := CallableReturn(typ.NewUnion(
		callableReturning(returns[0]),
		callableReturning(returns[1]),
		callableReturning(returns[2]),
	))
	if !ok {
		t.Fatal("CallableReturn(union) failed")
	}
	assertType(t, got, normalize.UnionForProjection(returns...))
}

func TestCallableReturnUnionProjectionPolicy(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	tests := []struct {
		name     string
		receiver typ.Type
		want     typ.Type
	}{
		{
			name: "any absorbs concrete projection",
			receiver: typ.NewUnion(
				callableReturning(typ.Any),
				callableReturning(typ.String),
			),
			want: typ.Any,
		},
		{
			name: "optional return preserves nilability",
			receiver: typ.NewUnion(
				callableReturning(typ.NewOptional(typ.Number)),
				callableReturning(typ.String),
			),
			want: typ.NewUnion(typ.Nil, typ.String, typ.Number),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CallableReturn(tt.receiver)
			if !ok {
				t.Fatal("CallableReturn(union) failed")
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
