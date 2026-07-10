package branchcond

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticLuaTruthiness(t *testing.T) {
	tests := []struct {
		name   string
		expr   ast.Expr
		want   bool
		wantOK bool
	}{
		{name: "nil is falsy", expr: &ast.NilExpr{}, want: false, wantOK: true},
		{name: "false is falsy", expr: &ast.FalseExpr{}, want: false, wantOK: true},
		{name: "true is truthy", expr: &ast.TrueExpr{}, want: true, wantOK: true},
		{name: "number zero is truthy", expr: &ast.NumberExpr{Value: "0"}, want: true, wantOK: true},
		{name: "empty string is truthy", expr: &ast.StringExpr{Value: ""}, want: true, wantOK: true},
		{name: "table is truthy", expr: &ast.TableExpr{}, want: true, wantOK: true},
		{name: "function is truthy", expr: &ast.FunctionExpr{}, want: true, wantOK: true},
		{name: "not nil is truthy", expr: &ast.UnaryNotOpExpr{Expr: &ast.NilExpr{}}, want: true, wantOK: true},
		{name: "not string is falsy", expr: &ast.UnaryNotOpExpr{Expr: &ast.StringExpr{Value: "x"}}, want: false, wantOK: true},
		{name: "false and unknown stays falsy", expr: &ast.LogicalOpExpr{Operator: "and", Lhs: &ast.FalseExpr{}, Rhs: &ast.IdentExpr{Value: "x"}}, want: false, wantOK: true},
		{name: "true and false is falsy", expr: &ast.LogicalOpExpr{Operator: "and", Lhs: &ast.TrueExpr{}, Rhs: &ast.FalseExpr{}}, want: false, wantOK: true},
		{name: "true or unknown stays truthy", expr: &ast.LogicalOpExpr{Operator: "or", Lhs: &ast.TrueExpr{}, Rhs: &ast.IdentExpr{Value: "x"}}, want: true, wantOK: true},
		{name: "nil or string is truthy", expr: &ast.LogicalOpExpr{Operator: "or", Lhs: &ast.NilExpr{}, Rhs: &ast.StringExpr{Value: "fallback"}}, want: true, wantOK: true},
		{name: "wrapped expression is unwrapped", expr: &ast.NonNilAssertExpr{Expr: &ast.CastExpr{Expr: &ast.FalseExpr{}}}, want: false, wantOK: true},
		{name: "unknown identifier", expr: &ast.IdentExpr{Value: "x"}, wantOK: false},
		{name: "unknown logical rhs", expr: &ast.LogicalOpExpr{Operator: "and", Lhs: &ast.TrueExpr{}, Rhs: &ast.IdentExpr{Value: "x"}}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := StaticLuaTruthiness(tt.expr)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("truthiness = %v, want %v", got, tt.want)
			}
		})
	}
}
