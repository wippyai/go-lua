package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyStepsProjectsGenericArgAndCallableReturn(t *testing.T) {
	callback := typ.Func().Returns(typ.Number).Build()
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	source := typ.Instantiate(channel, callback)

	got := ApplySteps(source, []effect.TypeProjectionStep{
		effect.ProjectGenericArg(0),
		effect.ProjectCallableReturn(),
	})
	if !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("projected type = %v, want number", got)
	}
}

func TestApplyStepsInstantiatesGenericFromMetaPayload(t *testing.T) {
	channel := typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
	meta := typ.NewMeta(typ.String)

	got := ApplySteps(meta, []effect.TypeProjectionStep{
		effect.ProjectInstantiateGeneric(channel),
	})
	want := typ.Instantiate(channel, typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("projected type = %v, want %v", got, want)
	}
}

func TestFromArgsResolvesNegativeParamRef(t *testing.T) {
	projection := effect.TypeProjection{
		Source: effect.ParamRef{Index: -1},
		Steps:  []effect.TypeProjectionStep{effect.ProjectGenericArg(0)},
	}
	box := typ.NewGeneric("Box", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Box", nil))

	got := FromArgs([]typ.Type{typ.Boolean, typ.Instantiate(box, typ.String)}, projection)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("projected type = %v, want string", got)
	}
}
