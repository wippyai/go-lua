package effects

import (
	"testing"

	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestHasEffectInType_Function(t *testing.T) {
	fn := typ.Func().Effects(effect.WithCallableType()).Build()
	if !HasEffectInType(fn, effect.Row.HasCallableType) {
		t.Error("expected callable type effect on function")
	}
}

func TestHasEffectInType_Union(t *testing.T) {
	fn := typ.Func().Effects(effect.WithCallableType()).Build()
	u := typ.NewUnion(fn, typ.String)
	if !HasEffectInType(u, effect.Row.HasCallableType) {
		t.Error("expected callable type effect on union member")
	}
}

func TestHasEffectInType_Optional(t *testing.T) {
	fn := typ.Func().Effects(effect.WithCallableType()).Build()
	opt := typ.NewOptional(fn)
	if !HasEffectInType(opt, effect.Row.HasCallableType) {
		t.Error("expected callable type effect on optional inner")
	}
}

func TestHasEffectInType_NoEffect(t *testing.T) {
	if HasEffectInType(typ.String, effect.Row.HasCallableType) {
		t.Error("expected no effect for non-function type")
	}
}

func TestCallableTypeForMeta(t *testing.T) {
	meta := typ.NewMeta(typ.String)
	fn := CallableTypeForMeta(meta)
	if fn == nil {
		t.Fatal("expected callable type for meta")
	}
	if len(fn.Returns) != 1 || fn.Returns[0] != typ.String {
		t.Error("expected callable type to return meta.Of")
	}
	if !HasEffectInType(fn, effect.Row.HasCallableType) {
		t.Error("expected callable type effect on meta function")
	}
}
