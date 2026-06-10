package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestApplyTypeProjectionField(t *testing.T) {
	source := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, ok := ApplyTypeProjection(source, returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectField("name")},
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

	got, ok := ApplyTypeProjection(source, returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectCallableReturn()},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection callable return failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestApplyTypeProjectionGenericArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	got, ok := ApplyTypeProjection(typ.NewAlias("StringBox", typ.Instantiate(box, typ.String)), returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectGenericArg(0)},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection generic arg failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyTypeProjectionGenericArgRejectsMissingArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	if got, ok := ApplyTypeProjection(typ.Instantiate(box, typ.String), returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectGenericArg(1)},
	}); ok || got != nil {
		t.Fatal("ApplyTypeProjection generic missing arg succeeded")
	}
}

func TestApplyTypeProjectionInstantiateGeneric(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := ApplyTypeProjection(typ.NewMeta(typ.String), returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectInstantiateGeneric(channel)},
	})
	if !ok {
		t.Fatal("ApplyTypeProjection instantiate generic failed")
	}
	assertProjectionType(t, got, typ.Instantiate(channel, typ.String))
}

func TestApplyTypeProjectionInstantiateGenericRejectsConstraintMismatch(t *testing.T) {
	param := typ.NewTypeParam("T", typ.Number)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	if got, ok := ApplyTypeProjection(typ.NewMeta(typ.String), returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{returns.ProjectInstantiateGeneric(channel)},
	}); ok || got != nil {
		t.Fatal("ApplyTypeProjection instantiate generic constraint mismatch succeeded")
	}
}

func TestApplyTypeProjectionChainFieldCallableReturn(t *testing.T) {
	source := typ.NewRecord().
		Field("make", typ.Func().Returns(typ.Boolean).Build()).
		Build()

	got, ok := ApplyTypeProjection(source, returns.TypeProjection{
		Steps: []returns.TypeProjectionStep{
			returns.ProjectField("make"),
			returns.ProjectCallableReturn(),
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
