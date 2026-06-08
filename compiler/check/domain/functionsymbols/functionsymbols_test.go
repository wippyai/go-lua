package functionsymbols

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromSymbolsNormalizesBoundarySet(t *testing.T) {
	got := FromSymbols(3, 0, 1, 3).Slice()
	if !slices.Equal(got, []ccfg.SymbolID{1, 3}) {
		t.Fatalf("FromSymbols().Slice() = %v, want [1 3]", got)
	}
}

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

func TestParametersAndCurrentFunctionUseGraphBoundaryIdentity(t *testing.T) {
	graph, nested := boundaryGraph(t, `
local function recur(value)
	if value then
		return recur(value)
	end
end
`)
	childGraph := ccfg.BuildWithBindings(nested, graph.Bindings())
	param := firstParamSymbol(t, childGraph)

	if got := Parameters(childGraph); !got.Contains(param) {
		t.Fatalf("Parameters() = %#v, want parameter symbol %d", got.Slice(), param)
	}
	if got := CurrentFunction(graph, nested); !got.Contains(nestedFunctionSymbol(t, graph, nested)) {
		t.Fatalf("CurrentFunction() = %#v, want local function symbol", got.Slice())
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

func firstParamSymbol(t *testing.T, graph *ccfg.Graph) ccfg.SymbolID {
	t.Helper()
	for _, slot := range graph.ParamSlotsReadOnly() {
		if slot.Symbol != 0 {
			return slot.Symbol
		}
	}
	t.Fatal("expected parameter symbol")
	return 0
}

func nestedFunctionSymbol(t *testing.T, graph *ccfg.Graph, fn *ast.FunctionExpr) ccfg.SymbolID {
	t.Helper()
	for _, nested := range graph.NestedFunctions() {
		if nested.Func == fn && nested.Symbol != 0 {
			return nested.Symbol
		}
	}
	t.Fatal("expected nested function symbol")
	return 0
}
