package storage

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestKeyKindRejectsTypedNilUnaryMinus(t *testing.T) {
	var unary *ast.UnaryMinusOpExpr
	if got := keyKind(unary); got != flowkind.FieldKey {
		t.Fatalf("keyKind(typed-nil unary) = %d, want FieldKey", got)
	}
}

func TestKeyKindRejectsTypedNilUnaryMinusOperand(t *testing.T) {
	var number *ast.NumberExpr
	if got := keyKind(&ast.UnaryMinusOpExpr{Expr: number}); got != flowkind.FieldKey {
		t.Fatalf("keyKind(unary with typed-nil number) = %d, want FieldKey", got)
	}
}

func TestKeyKindRejectsTypedNilScalarLiterals(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{name: "nil", expr: (*ast.NilExpr)(nil)},
		{name: "true", expr: (*ast.TrueExpr)(nil)},
		{name: "false", expr: (*ast.FalseExpr)(nil)},
		{name: "number", expr: (*ast.NumberExpr)(nil)},
		{name: "string", expr: (*ast.StringExpr)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := keyKind(test.expr); got != flowkind.FieldKey {
				t.Fatalf("keyKind(%s typed-nil literal) = %d, want FieldKey", test.name, got)
			}
		})
	}
}
