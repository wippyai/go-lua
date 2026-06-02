package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCollectFunctionTopologyExtractsNamedAndLocalFunctionSymbols(t *testing.T) {
	root := parseFactsTestChunk(t, `
local function named()
	return "named"
end

local local_fn = function()
	return "local"
end

local table_fn = {
	field = function()
		return "field"
	end,
}
`)
	graphs := collectFactsTestGraphs(root)
	rootGraph := graphs[root]
	want := expectedFunctionBindings(t, graphs, rootGraph)
	funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)

	entries := collectFunctionBindings(Program{
		Refs: factsTestRefs(graphs),
		Graph: func(r ref.FuncRef) *cfg.Graph {
			return graphByRef(graphs, r)
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
			r, ok := funcRefsBySymbol[sym]
			return r, ok
		},
	})

	got := map[string]ref.FuncRef{}
	for _, entry := range entries {
		name := rootGraph.Bindings().Name(entry.Symbol)
		if name == "" {
			continue
		}
		got[name] = entry.FuncRef
	}
	for name, wantRef := range want {
		if gotRef, ok := got[name]; !ok || gotRef != wantRef {
			t.Fatalf("binding %q = %+v/%v, want %+v/true; entries=%+v", name, gotRef, ok, wantRef, entries)
		}
	}
	if _, ok := got["table_fn.field"]; ok {
		t.Fatalf("field function leaked into root symbol bindings: %+v", entries)
	}
}

func TestBuildPreTransferStoresFunctionBindings(t *testing.T) {
	root := parseFactsTestChunk(t, `
local function named()
	return "named"
end
`)
	graphs := collectFactsTestGraphs(root)
	rootGraph := graphs[root]
	want := expectedFunctionBindings(t, graphs, rootGraph)
	funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)

	m := BuildPreTransfer(Program{
		Refs: factsTestRefs(graphs),
		Graph: func(r ref.FuncRef) *cfg.Graph {
			return graphByRef(graphs, r)
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
			r, ok := funcRefsBySymbol[sym]
			return r, ok
		},
	})

	refs := m.FunctionBindings()
	if len(refs) == 0 {
		t.Fatal("BuildPreTransfer did not store function bindings")
	}
	for name, wantRef := range want {
		sym := symbolNamed(t, rootGraph, name)
		if gotRef, ok := m.FunctionRef(sym); !ok || gotRef != wantRef {
			t.Fatalf("FunctionRef(%q/%d) = %+v/%v, want %+v/true", name, sym, gotRef, ok, wantRef)
		}
	}
	refs[0].FuncRef = ref.FuncRef{}
	if again := m.FunctionBindings(); again[0].FuncRef == (ref.FuncRef{}) {
		t.Fatalf("FunctionBindings exposed mutable backing store: %+v", again)
	}
}

func TestFunctionBindingTypesProjectsThroughSignatureResolver(t *testing.T) {
	refA := ref.FuncRef{GraphID: 10}
	refB := ref.FuncRef{GraphID: 20}
	refC := ref.FuncRef{GraphID: 30}
	m := Module{functionBindings: []topology.FunctionBinding{
		{Symbol: cfg.SymbolID(3), FuncRef: refA},
		{Symbol: cfg.SymbolID(5), FuncRef: refB},
		{Symbol: cfg.SymbolID(7), FuncRef: refC},
	}}

	got := m.FunctionBindingTypes(func(r ref.FuncRef) typ.Type {
		switch r {
		case refA:
			return typ.Func().Returns(typ.String).Build()
		case refB:
			return typ.Unknown
		default:
			return nil
		}
	})

	if len(got) != 1 {
		t.Fatalf("FunctionBindingTypes size = %d (%v), want 1", len(got), got)
	}
	if !typ.TypeEquals(got[cfg.SymbolID(3)], typ.Func().Returns(typ.String).Build()) {
		t.Fatalf("FunctionBindingTypes[3] = %v, want string-return function", got[cfg.SymbolID(3)])
	}
	if got[cfg.SymbolID(5)] != nil || got[cfg.SymbolID(7)] != nil {
		t.Fatalf("FunctionBindingTypes kept absent/unknown signatures: %v", got)
	}
}

func expectedFunctionBindings(t *testing.T, graphs map[*ast.FunctionExpr]*cfg.Graph, rootGraph *cfg.Graph) map[string]ref.FuncRef {
	t.Helper()
	want := map[string]ref.FuncRef{}
	rootGraph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
			return
		}
		name := rootGraph.Bindings().Name(info.Symbol)
		if name == "named" {
			want[name] = refForFactsGraph(t, graphs, info.FuncExpr)
		}
	})
	for _, lfa := range rootGraph.LocalFunctionAssignments() {
		if lfa.Symbol == 0 || lfa.Func == nil {
			continue
		}
		name := rootGraph.Bindings().Name(lfa.Symbol)
		if name == "local_fn" || name == "named" {
			want[name] = refForFactsGraph(t, graphs, lfa.Func)
		}
	}
	if len(want) == 0 {
		t.Fatal("test fixture produced no expected function bindings")
	}
	return want
}

func refForFactsGraph(t *testing.T, graphs map[*ast.FunctionExpr]*cfg.Graph, fn *ast.FunctionExpr) ref.FuncRef {
	t.Helper()
	g := graphs[fn]
	if g == nil {
		t.Fatalf("missing graph for function literal %#v", fn)
	}
	return ref.FuncRef{GraphID: g.ID()}
}

func symbolNamed(t *testing.T, g *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	for _, sym := range g.Bindings().SymbolsByName(name) {
		if sym != 0 {
			return sym
		}
	}
	t.Fatalf("missing symbol named %q", name)
	return 0
}
