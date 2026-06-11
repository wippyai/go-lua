package valueexpr

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestTopLevelProducerLooksThroughAssertionWrappers(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "foo"}}
	wrapped := &ast.NonNilAssertExpr{
		Expr: &ast.CastExpr{
			Expr: call,
			Type: &ast.PrimitiveTypeExpr{Name: "number"},
		},
	}

	got, ok := Call(wrapped)
	if !ok || got != call {
		t.Fatalf("Call(wrapped) = %p/%v, want inner call %p/true", got, ok, call)
	}
	if !CanProduceMultipleValues(wrapped) {
		t.Fatal("wrapped call did not produce multiple values")
	}
}

func TestAdjustRetUsesInnerProducer(t *testing.T) {
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "foo"}, AdjustRet: true}
	wrappedCall := &ast.CastExpr{Expr: call, Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	if !AdjustRet(wrappedCall) {
		t.Fatal("wrapped adjusted call did not report AdjustRet")
	}

	vararg := &ast.Comma3Expr{AdjustRet: true}
	wrappedVararg := &ast.NonNilAssertExpr{Expr: vararg}
	if !AdjustRet(wrappedVararg) {
		t.Fatal("wrapped adjusted vararg did not report AdjustRet")
	}
}

func TestAssertionInnerStopsAtNonAssertion(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	wrapped := &ast.CastExpr{Expr: expr, Type: &ast.PrimitiveTypeExpr{Name: "any"}}

	if got := AssertionInner(wrapped); got != expr {
		t.Fatalf("AssertionInner = %T %p, want ident %p", got, got, expr)
	}
}
