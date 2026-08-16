package signature

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestFunctionEqualsComparesTypeAndEffectSeparately(t *testing.T) {
	fn := typ.Func().
		Param("value", typ.String).
		Returns(typ.Boolean).
		Build()
	nonPure := Function{Type: fn, Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})}
	same := Function{Type: fn, Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})}
	pure := Function{Type: fn, Effect: effect.Empty}
	differentType := Function{
		Type: typ.Func().
			Param("value", typ.Number).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	if !nonPure.Equals(same) {
		t.Fatalf("equal signatures were not equal")
	}
	if nonPure.Equals(pure) {
		t.Fatalf("effect rows should be part of signature equality")
	}
	if nonPure.Equals(differentType) {
		t.Fatalf("function type should be part of signature equality")
	}
}

func TestFunctionCloneCopiesEffectRowAndFunctionSlices(t *testing.T) {
	original := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Open("rho", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	clone := original.Clone()
	if !clone.Equals(original) {
		t.Fatalf("clone = %v, want %v", clone, original)
	}
	if clone.Type == original.Type {
		t.Fatalf("clone should rebuild the function carrier type")
	}

	clone.Type.Params[0].Type = typ.Number
	clone.Effect.Tail.Name = "sigma"

	if original.Type.Params[0].Type != typ.String {
		t.Fatalf("clone mutation changed original function params")
	}
	if original.Effect.Tail.Name != "rho" {
		t.Fatalf("clone mutation changed original effect tail")
	}
}

func TestFunctionStringIncludesNonPureEffect(t *testing.T) {
	sig := Function{
		Type: typ.Func().
			Param("value", typ.String).
			Returns(typ.Boolean).
			Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}

	if got, want := sig.String(), "fun(value: string) -> boolean ! {errret(val[0], err[1])}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
