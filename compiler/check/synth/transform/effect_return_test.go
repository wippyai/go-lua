package transform

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyEffectTransform_ErrorReturnOptionalizes(t *testing.T) {
	spec := contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(spec).
		Build()

	got := ApplyEffectTransform(fn, nil, 0, typ.String)
	want := typ.NewOptional(typ.String)

	if !typ.TypeEquals(got, want) {
		t.Fatalf("ApplyEffectTransform error return: got %v, want %v", got, want)
	}
}

func TestBuildSelectResultUnion_ResolvesNegativeCasesIndex(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	args := []typ.Type{
		typ.String,
		typ.NewArray(typ.Instantiate(selectCase, typ.Integer, typ.Boolean)),
	}
	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: -1},
		Default: effect.ParamRef{Index: -1},
	})

	want := typ.NewRecord().
		Field("channel", typ.Integer).
		Field("ok", typ.Boolean).
		Field("value", typ.Boolean).
		Build()

	if !typ.TypeEquals(got, want) {
		t.Fatalf("buildSelectResultUnion negative cases index: got %v, want %v", got, want)
	}
}

func TestBuildSelectResultUnion_DefaultFieldRequiresInRangeDefaultArg(t *testing.T) {
	chParam := typ.NewTypeParam("Ch", typ.Any)
	valParam := typ.NewTypeParam("T", typ.Any)
	selectCase := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{chParam, valParam}, typ.NewRecord().Build())

	args := []typ.Type{
		typ.NewArray(typ.Instantiate(selectCase, typ.String, typ.Number)),
	}
	got := buildSelectResultUnion(args, effect.SelectResultOfCases{
		Cases:   effect.ParamRef{Index: 0},
		Default: effect.ParamRef{Index: 1},
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", got)
	}
	if field := rec.GetField("default"); field != nil {
		t.Fatalf("did not expect default field when default arg is absent, got %+v", *field)
	}
}
