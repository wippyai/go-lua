package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEffectFromType_Nil(t *testing.T) {
	if EffectFromType(nil) != nil {
		t.Error("expected nil for nil type")
	}
}

func TestEffectFromType_NonFunction(t *testing.T) {
	if EffectFromType(typ.String) != nil {
		t.Error("expected nil for non-function type")
	}
}

func TestEffectFromType_PureFunction(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	result := EffectFromType(fn)
	if result != nil {
		t.Errorf("expected nil for pure function, got %v", result)
	}
}

func TestEffectFromType_WithEffects(t *testing.T) {
	row := effect.Empty.With(effect.IO{})
	fn := typ.Func().Returns(typ.String).Effects(row).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for function with effects")
	}
}

func TestEffectFromType_NeverReturn(t *testing.T) {
	fn := typ.Func().Returns(typ.Never).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for never-returning function")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true for never-returning function")
	}
}

func TestEffectFromType_WithRefinement(t *testing.T) {
	eff := &constraint.FunctionRefinement{Terminates: true}
	fn := typ.Func().Returns(typ.String).WithRefinement(eff).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected refinement to be returned")
	}
}
