package valueexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
		{name: "wrapped int", expr: wrappedExpr(&ast.NumberExpr{Value: "0x10"}), want: typ.LiteralInt(16)},
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

func TestLiteralTypeUnwrapsAssertionAndCast(t *testing.T) {
	expr := &ast.CastExpr{Expr: &ast.NonNilAssertExpr{Expr: &ast.StringExpr{Value: "wrapped"}}}
	got, ok := LiteralType(expr)
	if !ok {
		t.Fatal("LiteralType returned false")
	}
	if !got.Equals(typ.LiteralString("wrapped")) {
		t.Fatalf("LiteralType = %v, want %v", got, typ.LiteralString("wrapped"))
	}
	if inner := sourceprovenance.AssertionInner(expr); inner != expr.Expr.(*ast.NonNilAssertExpr).Expr {
		t.Fatalf("AssertionInner = %T, want *ast.StringExpr", inner)
	}
}

func TestLiteralTypeDoesNotUseAnyCastAsProof(t *testing.T) {
	expr := &ast.CastExpr{
		Expr: &ast.StringExpr{Value: "wrapped"},
		Type: &ast.PrimitiveTypeExpr{Name: "any"},
	}

	if got, ok := LiteralType(expr); ok || got != nil {
		t.Fatalf("LiteralType(any cast) = %v/%v, want nil/false", got, ok)
	}
}

func TestLiteralTypeDoesNotUseUnknownCastAsProof(t *testing.T) {
	expr := &ast.CastExpr{
		Expr: &ast.StringExpr{Value: "wrapped"},
		Type: &ast.PrimitiveTypeExpr{Name: "unknown"},
	}

	if got, ok := LiteralType(expr); ok || got != nil {
		t.Fatalf("LiteralType(unknown cast) = %v/%v, want nil/false", got, ok)
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
		{name: "wrapped table", expr: wrappedExpr(&ast.TableExpr{}), want: runtimekind.Singleton(runtimekind.Table)},
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

func TestRuntimeKindUnwrapsAssertionAndCast(t *testing.T) {
	expr := &ast.NonNilAssertExpr{Expr: &ast.CastExpr{Expr: &ast.FunctionExpr{}}}
	got, ok := RuntimeKind(expr)
	if !ok {
		t.Fatal("RuntimeKind returned false")
	}
	want := runtimekind.Singleton(runtimekind.Function)
	if !runtimekind.Equal(got, want) {
		t.Fatalf("RuntimeKind = %v, want %v", got, want)
	}
	if inner := sourceprovenance.AssertionInner(expr); inner != expr.Expr.(*ast.CastExpr).Expr {
		t.Fatalf("AssertionInner = %T, want *ast.FunctionExpr", inner)
	}
}

func TestTypeValueRefPartsRecognizesDottedRefs(t *testing.T) {
	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object:    &ast.IdentExpr{Value: "protocol"},
			Key:       &ast.StringExpr{Value: "events"},
			KeySyntax: ast.AttrKeyDot,
		},
		Key:       &ast.StringExpr{Value: "Message"},
		KeySyntax: ast.AttrKeyDot,
	}

	got, ok := TypeValueRefParts(expr)

	if !ok {
		t.Fatal("TypeValueRefParts returned false")
	}
	want := []string{"protocol", "events", "Message"}
	if len(got) != len(want) {
		t.Fatalf("TypeValueRefParts length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TypeValueRefParts[%d] = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}
}

func TestTypeValueRefPartsRejectsDynamicOrWrappedRefs(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
	}{
		{name: "empty ident", expr: &ast.IdentExpr{}},
		{name: "dynamic index", expr: &ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "protocol"}, Key: &ast.IdentExpr{Value: "name"}, KeySyntax: ast.AttrKeyIndex}},
		{name: "empty dot key", expr: &ast.AttrGetExpr{Object: &ast.IdentExpr{Value: "protocol"}, Key: &ast.StringExpr{}, KeySyntax: ast.AttrKeyDot}},
		{name: "wrapped", expr: &ast.NonNilAssertExpr{Expr: &ast.IdentExpr{Value: "Result"}}},
	}

	for _, tt := range tests {
		if got, ok := TypeValueRefParts(tt.expr); ok || got != nil {
			t.Fatalf("%s: TypeValueRefParts = %#v/%v, want nil/false", tt.name, got, ok)
		}
	}
}

func wrappedExpr(inner ast.Expr) ast.Expr {
	return &ast.CastExpr{Expr: &ast.NonNilAssertExpr{Expr: inner}}
}
