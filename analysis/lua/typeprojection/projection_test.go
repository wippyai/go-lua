package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestApplyField(t *testing.T) {
	source := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{projection.Field("name")},
	})
	if !ok {
		t.Fatal("Apply field failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyCallableReturn(t *testing.T) {
	source := typ.Func().
		Param("value", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{projection.CallableReturn()},
	})
	if !ok {
		t.Fatal("Apply callable return failed")
	}
	assertProjectionType(t, got, typ.Number)
}

func TestApplyGenericArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	got, ok := Apply(typ.NewAlias("StringBox", typ.Instantiate(box, typ.String)), projection.Projection{
		Steps: []projection.Step{projection.GenericArg(0)},
	})
	if !ok {
		t.Fatal("Apply generic arg failed")
	}
	assertProjectionType(t, got, typ.String)
}

func TestApplyGenericArgRejectsMissingArg(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)

	if got, ok := Apply(typ.Instantiate(box, typ.String), projection.Projection{
		Steps: []projection.Step{projection.GenericArg(1)},
	}); ok || got != nil {
		t.Fatal("Apply generic missing arg succeeded")
	}
}

func TestApplyInstantiateGeneric(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	got, ok := Apply(typ.NewMeta(typ.String), projection.Projection{
		Steps: []projection.Step{projection.InstantiateGeneric(channel)},
	})
	if !ok {
		t.Fatal("Apply instantiate generic failed")
	}
	assertProjectionType(t, got, typ.Instantiate(channel, typ.String))
}

func TestApplyInstantiateGenericRejectsConstraintMismatch(t *testing.T) {
	param := typ.NewTypeParam("T", typ.Number)
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))

	if got, ok := Apply(typ.NewMeta(typ.String), projection.Projection{
		Steps: []projection.Step{projection.InstantiateGeneric(channel)},
	}); ok || got != nil {
		t.Fatal("Apply instantiate generic constraint mismatch succeeded")
	}
}

func TestApplyChainFieldCallableReturn(t *testing.T) {
	source := typetable.NewRecord().
		Field("make", typ.Func().Returns(typ.Boolean).Build()).
		Build()

	got, ok := Apply(source, projection.Projection{
		Steps: []projection.Step{
			projection.Field("make"),
			projection.CallableReturn(),
		},
	})
	if !ok {
		t.Fatal("Apply field callable return chain failed")
	}
	assertProjectionType(t, got, typ.Boolean)
}

func assertProjectionType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
