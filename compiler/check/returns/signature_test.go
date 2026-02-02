package returns

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildFunctionSignatureWithSummary(t *testing.T) {
	t.Run("nil sig returns nil", func(t *testing.T) {
		result := BuildFunctionSignatureWithSummary(nil, nil)
		if result != nil {
			t.Error("expected nil")
		}
	})

	t.Run("sig with returns preserved", func(t *testing.T) {
		sig := typ.Func().Returns(typ.String).Build()
		result := BuildFunctionSignatureWithSummary(sig, []typ.Type{typ.Number})
		if len(result.Returns) != 1 || result.Returns[0] != typ.String {
			t.Error("expected original returns to be preserved")
		}
	})

	t.Run("empty summary gets unknown", func(t *testing.T) {
		sig := typ.Func().Build()
		result := BuildFunctionSignatureWithSummary(sig, nil)
		if len(result.Returns) != 1 || result.Returns[0] != typ.Unknown {
			t.Error("expected unknown return")
		}
	})
}

func TestBuildFunctionTypeFromSummary(t *testing.T) {
	t.Run("empty returns unknown", func(t *testing.T) {
		result := BuildFunctionTypeFromSummary(nil)
		fn, ok := result.(*typ.Function)
		if !ok || len(fn.Returns) != 1 || fn.Returns[0] != typ.Unknown {
			t.Error("expected function with unknown return")
		}
	})

	t.Run("with returns", func(t *testing.T) {
		result := BuildFunctionTypeFromSummary([]typ.Type{typ.Number, typ.String})
		fn, ok := result.(*typ.Function)
		if !ok || len(fn.Returns) != 2 {
			t.Error("expected function with 2 returns")
		}
	})
}
