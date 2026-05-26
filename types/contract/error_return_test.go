package contract

import (
	"testing"

	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestErrorReturnForValue_ShapeOnlyDoesNotInferCorrelation(t *testing.T) {
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Build()

	if got := ErrorReturnForValue(fn, 0); got != nil {
		t.Fatalf("shape-only (value, err?) must not infer correlation, got %+v", *got)
	}
}

func TestErrorReturnForValue_PlainTupleHasNoCorrelation(t *testing.T) {
	fn := typ.Func().
		Returns(typ.Number, typ.String).
		Build()

	if got := ErrorReturnForValue(fn, 0); got != nil {
		t.Fatalf("plain tuple returns must not infer error correlation, got %+v", *got)
	}
}

func TestErrorReturnForValue_ExplicitLabelWins(t *testing.T) {
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 1, ErrorIndex: 0})).
		Build()

	if got := ErrorReturnForValue(fn, 0); got != nil {
		t.Fatalf("did not expect convention when explicit label targets another slot, got %+v", *got)
	}
	got := ErrorReturnForValue(fn, 1)
	if got == nil || *got != (effect.ErrorReturn{ValueIndex: 1, ErrorIndex: 0}) {
		t.Fatalf("expected explicit label, got %+v", got)
	}
}

func TestErrorReturnForValue_CorrelatedReturnSuppressesConvention(t *testing.T) {
	fn := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(NewSpec().WithEffects(effect.CorrelatedReturn{Indices: []int{0, 1}})).
		Build()

	if got := ErrorReturnForValue(fn, 0); got != nil {
		t.Fatalf("did not expect error return with correlated-return spec, got %+v", *got)
	}
}
