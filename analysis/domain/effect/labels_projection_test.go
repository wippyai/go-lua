package effect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestApplyTypeProjectionField(t *testing.T) {
	source := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, ok := ApplyTypeProjection(source, TypeProjection{
		Steps: []TypeProjectionStep{ProjectField("name")},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection field failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyTypeProjectionCallableReturn(t *testing.T) {
	source := typ.Func().
		Param("value", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := ApplyTypeProjection(source, TypeProjection{
		Steps: []TypeProjectionStep{ProjectCallableReturn()},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection callable return failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestApplyTypeProjectionGenericArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	got, ok := ApplyTypeProjection(typ.Instantiate(box, typ.String), TypeProjection{
		Steps: []TypeProjectionStep{ProjectGenericArg(0)},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection generic arg failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyTypeProjectionInstantiateGeneric(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := ApplyTypeProjection(typ.NewMeta(typ.String), TypeProjection{
		Steps: []TypeProjectionStep{ProjectInstantiateGeneric(channel)},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection instantiate generic failed")
	}
	assertProjectionType(t, got, typ.Instantiate(channel, typ.String))
}

func TestApplyTypeProjectionChainFieldCallableReturn(t *testing.T) {
	source := typ.NewRecord().
		Field("make", typ.Func().Returns(typ.Boolean).Build()).
		Build()

	got, ok := ApplyTypeProjection(source, TypeProjection{
		Steps: []TypeProjectionStep{
			ProjectField("make"),
			ProjectCallableReturn(),
		},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection field callable return chain failed")
	}
	assertProjectionType(t, got, typ.Boolean)
}

func assertProjectionType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
