package iteration

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
)

func TestKeyedSource(t *testing.T) {
	source := ident("items")
	iter := &ast.FuncCallExpr{Args: []ast.Expr{source}}

	got, ok := KeyedSource(iter, func(*ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
		return effect.IterateKeyed, 0, true
	})
	if !ok || got != source {
		t.Fatalf("KeyedSource() = (%#v, %v), want source/true", got, ok)
	}

	if _, ok := KeyedSource(iter, func(*ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
		return effect.IterateIndexed, 0, true
	}); ok {
		t.Fatal("KeyedSource accepted indexed iterator")
	}
}

func TestIndexedSourceSymbol(t *testing.T) {
	bindings := bind.NewBindingTable()
	names := bindIdent(bindings, ident("names"), 10)
	iter := call("ipairs", names)

	got, ok := IndexedSourceSymbol(iter, bindings, indexedSource0)
	if !ok || got != 10 {
		t.Fatalf("IndexedSourceSymbol() = (%d, %v), want 10/true", got, ok)
	}

	if _, ok := IndexedSourceSymbol(iter, bindings, func(*ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
		return effect.IterateKeyed, 0, true
	}); ok {
		t.Fatal("IndexedSourceSymbol accepted keyed iterator")
	}
}

func TestIndexedSourcePathStaticField(t *testing.T) {
	bindings := bind.NewBindingTable()
	graph := bindIdent(bindings, ident("graph"), 20)
	routes := &ast.AttrGetExpr{
		Object:    graph,
		Key:       &ast.StringExpr{Value: "pending_routes"},
		KeySyntax: ast.AttrKeyDot,
	}
	iter := call("ipairs", routes)

	got, ok := IndexedSourcePath(iter, bindings, indexedSource0)
	want := constraint.NewPath(20, "graph").Field("pending_routes")
	if !ok || !got.Equal(want) {
		t.Fatalf("IndexedSourcePath() = (%v, %v), want %v/true", got, ok, want)
	}
}

func TestContainerPath_StaticField(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := bindIdent(bindings, ident("container"), 20)
	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "inner"},
	}
	got, ok := ContainerPath(expr, bindings)
	want := constraint.NewPath(20, "container").Field("inner")
	if !ok || !got.Equal(want) {
		t.Fatalf("ContainerPath() = (%v, %v), want %v/true", got, ok, want)
	}
}

func indexedSource0(*ast.FuncCallExpr) (effect.IteratorKind, int, bool) {
	return effect.IterateIndexed, 0, true
}

func call(name string, args ...ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name), Args: args}
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func bindIdent(table *bind.BindingTable, id *ast.IdentExpr, sym cfg.SymbolID) *ast.IdentExpr {
	table.Bind(id, sym)
	table.SetName(sym, id.Value)
	return id
}
