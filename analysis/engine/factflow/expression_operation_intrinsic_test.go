package factflow

import (
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"testing"
)

func TestExpressionIntrinsicCannotBeForgedByUnarySpelling(t *testing.T) {
	shape, _ := NewValueSourceShape(true, false, false, false)
	arg, _ := NewStringLiteralValueSource("x", 0, 0, 0, shape)
	if _, ok := NewUnaryExpressionOperation("lua_type", arg); ok {
		t.Fatal("forged lua_type unary accepted")
	}
	op, ok := NewIntrinsicExpressionOperation(intrinsic.LuaType, arg)
	got, exact := op.Intrinsic()
	if !ok || !exact || got != intrinsic.LuaType || op.Op() != "" {
		t.Fatalf("typed intrinsic = %v/%v/%v", got, exact, ok)
	}
}
