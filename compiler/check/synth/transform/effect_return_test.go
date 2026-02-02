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
