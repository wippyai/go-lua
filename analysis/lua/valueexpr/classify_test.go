package valueexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLiteralTypeRecognizesObviousLiterals(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want typ.Type
	}{
		{name: "nil", expr: &ast.NilExpr{}, want: typ.Nil},
		{name: "true", expr: &ast.TrueExpr{}, want: typ.LiteralBool(true)},
		{name: "false", expr: &ast.FalseExpr{}, want: typ.LiteralBool(false)},
		{name: "string", expr: &ast.StringExpr{Value: "hello"}, want: typ.LiteralString("hello")},
		{name: "integer", expr: &ast.NumberExpr{Value: "42"}, want: typ.LiteralInt(42)},
		{name: "float", expr: &ast.NumberExpr{Value: "3.5"}, want: typ.LiteralNumber(3.5)},
		{name: "wrapped int", expr: &ast.NonNilAssertExpr{Expr: &ast.CastExpr{Expr: &ast.NumberExpr{Value: "0x10"}}}, want: typ.LiteralInt(16)},
	}

	for _, tt := range tests {
		got, ok := LiteralType(tt.expr)
		if !ok {
			t.Fatalf("%s: LiteralType returned false", tt.name)
		}
		if tt.want == typ.Nil {
			if got != typ.Nil {
				t.Fatalf("%s: LiteralType = %v, want nil", tt.name, got)
			}
			continue
		}
		if !got.Equals(tt.want) {
			t.Fatalf("%s: LiteralType = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLiteralTypeRejectsNonLiterals(t *testing.T) {
	tests := []ast.Expr{
		&ast.IdentExpr{Value: "x"},
		&ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
	}

	for i, expr := range tests {
		if got, ok := LiteralType(expr); ok || got != nil {
			t.Fatalf("case %d: LiteralType = %v/%v, want false/nil", i, got, ok)
		}
	}
}

func TestRuntimeKindRecognizesObviousRuntimeValues(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want runtimekind.Value
	}{
		{name: "nil", expr: &ast.NilExpr{}, want: runtimekind.Singleton(runtimekind.Nil)},
		{name: "bool", expr: &ast.TrueExpr{}, want: runtimekind.Singleton(runtimekind.Boolean)},
		{name: "number", expr: &ast.NumberExpr{Value: "7"}, want: runtimekind.Singleton(runtimekind.Number)},
		{name: "string", expr: &ast.StringExpr{Value: "hello"}, want: runtimekind.Singleton(runtimekind.String)},
		{name: "table", expr: &ast.TableExpr{}, want: runtimekind.Singleton(runtimekind.Table)},
		{name: "function", expr: &ast.FunctionExpr{}, want: runtimekind.Singleton(runtimekind.Function)},
		{name: "wrapped table", expr: &ast.NonNilAssertExpr{Expr: &ast.CastExpr{Expr: &ast.TableExpr{}}}, want: runtimekind.Singleton(runtimekind.Table)},
	}

	for _, tt := range tests {
		got, ok := RuntimeKind(tt.expr)
		if !ok {
			t.Fatalf("%s: RuntimeKind returned false", tt.name)
		}
		if !runtimekind.Equal(got, tt.want) {
			t.Fatalf("%s: RuntimeKind = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRuntimeKindRejectsNonObviousValues(t *testing.T) {
	tests := []ast.Expr{
		&ast.IdentExpr{Value: "x"},
		&ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
	}

	for i, expr := range tests {
		if got, ok := RuntimeKind(expr); ok || !got.IsBottom() {
			t.Fatalf("case %d: RuntimeKind = %v/%v, want false/bottom", i, got, ok)
		}
	}
}
