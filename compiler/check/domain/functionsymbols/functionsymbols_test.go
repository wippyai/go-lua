package functionsymbols

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFunctionBoundarySymbolsShareCapturedReturnedLocal(t *testing.T) {
	graph, nested := boundaryGraph(t, `
local owned = {}
local function child()
	return owned
end
return owned
`)
	childGraph := ccfg.BuildWithBindings(nested, graph.Bindings())
	captured := firstOwnedCapture(t, graph, nested)

	if got := OwnedCapturedByNested(graph); !got.Contains(captured) {
		t.Fatalf("OwnedCapturedByNested() = %#v, want captured symbol %d", got.Slice(), captured)
	}
	if got := Returned(graph); !got.Contains(captured) {
		t.Fatalf("Returned() = %#v, want returned symbol %d", got.Slice(), captured)
	}
	if got := CapturedFree(childGraph, nested); !got.Contains(captured) {
		t.Fatalf("CapturedFree() = %#v, want captured symbol %d", got.Slice(), captured)
	}
	if got := Captured(graph.Bindings(), nested); !got.Contains(captured) {
		t.Fatalf("Captured() = %#v, want captured symbol %d", got.Slice(), captured)
	}
	if got := NonGlobalCaptures(graph.Bindings(), nested); !got.Contains(captured) {
		t.Fatalf("NonGlobalCaptures() = %#v, want captured symbol %d", got.Slice(), captured)
	}
}

func boundaryGraph(t *testing.T, src string) (*ccfg.Graph, *ast.FunctionExpr) {
	t.Helper()
	stmts, err := parse.ParseString(src, "functionsymbols.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	graph := ccfg.Build(root)
	if graph == nil || graph.Bindings() == nil {
		t.Fatal("expected graph with bindings")
	}
	nested := graph.NestedFunctions()
	if len(nested) != 1 || nested[0].Func == nil {
		t.Fatalf("nested functions = %#v, want one function", nested)
	}
	return graph, nested[0].Func
}

func firstOwnedCapture(t *testing.T, graph *ccfg.Graph, fn *ast.FunctionExpr) ccfg.SymbolID {
	t.Helper()
	for _, sym := range graph.Bindings().CapturedSymbols(fn) {
		if graph.OwnsSymbol(sym) {
			return sym
		}
	}
	t.Fatal("expected nested function to capture an owned symbol")
	return 0
}
