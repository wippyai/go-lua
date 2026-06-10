package pathexpr

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.StringExpr{Value: name},
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.StringExpr{Value: key},
		KeySyntax: ast.AttrKeyIndex,
	}
}

func intIndex(obj ast.Expr, index string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       &ast.NumberExpr{Value: index},
		KeySyntax: ast.AttrKeyIndex,
	}
}

func dynamicIndex(obj ast.Expr, key ast.Expr) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: ast.AttrKeyIndex,
	}
}

func bindReturn(expr ast.Expr) *bind.Result {
	return bind.BindChunk([]ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{expr}}}, bind.Options{})
}

func mustResolvedRoot(t *testing.T, bindings *bind.Result, root *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(root)
	if !ok || id == 0 {
		t.Fatalf("SymbolOf(%q) = %d/%v, want non-zero symbol", root.Value, id, ok)
	}
	return id
}

func assertResolved(t *testing.T, expr ast.Expr, bindings *bind.Result, want path.Path) {
	t.Helper()
	got, ok := Resolve(expr, bindings)
	if !ok {
		t.Fatalf("Resolve(%T) rejected static path", expr)
	}
	if got.Root != want.Root || got.Symbol != want.Symbol || !reflect.DeepEqual(got.Segments, want.Segments) {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}

func assertRejected(t *testing.T, expr ast.Expr, bindings *bind.Result) {
	t.Helper()
	got, ok := Resolve(expr, bindings)
	if ok || !got.IsEmpty() {
		t.Fatalf("Resolve() = %#v/%v, want empty/false", got, ok)
	}
}

func TestResolveIdent(t *testing.T) {
	root := ident("value")
	bindings := bindReturn(root)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, root, bindings, path.NewPath(sym, "value"))
}

func TestResolveNestedDot(t *testing.T) {
	root := ident("obj")
	expr := dot(dot(root, "a"), "b")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").Field("a").Field("b"))
}

func TestResolveStringIndex(t *testing.T) {
	root := ident("obj")
	expr := stringIndex(root, "key")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").IndexStr("key"))
}

func TestResolveIntIndex(t *testing.T) {
	root := ident("obj")
	expr := intIndex(root, "12")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	assertResolved(t, expr, bindings, path.NewPath(sym, "obj").IndexInt(12))
}

func TestResolveMixedPath(t *testing.T) {
	root := ident("obj")
	expr := dot(intIndex(stringIndex(dot(root, "a"), "b"), "3"), "c")
	bindings := bindReturn(expr)
	sym := mustResolvedRoot(t, bindings, root)

	want := path.NewPath(sym, "obj").Field("a").IndexStr("b").IndexInt(3).Field("c")
	assertResolved(t, expr, bindings, want)
}

func TestResolveUnresolvedIdent(t *testing.T) {
	bound := ident("x")
	unresolved := ident("x")
	bindings := bindReturn(bound)

	assertRejected(t, unresolved, bindings)
}

func TestResolveRejectsDynamicIndex(t *testing.T) {
	root := ident("obj")
	key := ident("key")
	expr := dynamicIndex(root, key)
	bindings := bindReturn(expr)
	if _, ok := bindings.SymbolOf(key); !ok {
		t.Fatalf("test setup expected dynamic key ident to be bound")
	}

	assertRejected(t, expr, bindings)
}

func TestResolveNilBinding(t *testing.T) {
	assertRejected(t, ident("x"), nil)
	assertRejected(t, dot(ident("x"), "field"), nil)
}
