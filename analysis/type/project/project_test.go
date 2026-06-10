package project

import (
	"testing"

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

func TestGenericArgExtraction(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)
	inst := typ.Instantiate(box, typ.String)

	got, ok := GenericArg(typ.NewAlias("StringBox", inst), 0)
	if !ok {
		t.Fatal("GenericArg(alias Box<string>, 0) failed")
	}
	assertType(t, got, typ.String)

	if _, ok := GenericArg(inst, 1); ok {
		t.Fatal("GenericArg(Box<string>, 1) succeeded")
	}
}

func TestInstantiateGenericOneArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := InstantiateGeneric(channel, typ.NewMeta(typ.String))
	if !ok {
		t.Fatal("InstantiateGeneric(Channel<T>, typeof(string)) failed")
	}
	assertType(t, got, typ.Instantiate(channel, typ.String))
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
