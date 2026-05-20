package functionfact_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeForGraph_UsesCanonicalParentAndCache(t *testing.T) {
	st := store.NewSessionStore()
	fn := &ast.FunctionExpr{}
	graph := cfg.Build(fn)
	st.RegisterGraph(graph, fn)

	storedParent := scope.New().WithType("stored_parent", typ.String)
	defaultParent := scope.New().WithType("default_parent", typ.Number)
	registerGraphParent(t, st, graph, storedParent)

	sym := cfg.SymbolID(7)
	first := typ.Func().Returns(typ.String).Build()
	second := typ.Func().Returns(typ.Number).Build()
	writeFunctionFactType(st, graph, storedParent, sym, first)

	cache := functionfact.TypeCache{}
	if got := functionfact.TypeForGraph(st, graph, sym, defaultParent, cache); !typ.TypeEquals(got, first) {
		t.Fatalf("TypeForGraph() = %v, want %v", got, first)
	}

	writeFunctionFactType(st, graph, storedParent, sym, second)
	if got := functionfact.TypeForGraph(st, graph, sym, defaultParent, cache); !typ.TypeEquals(got, first) {
		t.Fatalf("cached TypeForGraph() = %v, want %v", got, first)
	}
	if got := functionfact.TypeForGraph(st, graph, sym, defaultParent, nil); !typ.TypeEquals(got, second) {
		t.Fatalf("uncached TypeForGraph() = %v, want %v", got, second)
	}
}

func TestTypeForSymbol_ResolvesOwningParentGraph(t *testing.T) {
	st := store.NewSessionStore()
	parentFn := &ast.FunctionExpr{}
	childFn := &ast.FunctionExpr{}
	parentGraph := cfg.Build(parentFn)
	childGraph := cfg.Build(childFn)
	st.RegisterGraph(parentGraph, parentFn)
	st.RegisterGraph(childGraph, childFn)

	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, parentGraph, parent)

	sym := cfg.SymbolID(11)
	fnType := typ.Func().Returns(typ.Boolean).Build()
	st.RegisterFunctionRef(sym, childFn, childGraph, parentGraph.ID(), 0)
	writeFunctionFactType(st, parentGraph, parent, sym, fnType)

	if got := functionfact.TypeForSymbol(st, sym, nil, functionfact.TypeCache{}); !typ.TypeEquals(got, fnType) {
		t.Fatalf("TypeForSymbol() = %v, want %v", got, fnType)
	}
}

func registerGraphParent(t *testing.T, st *store.SessionStore, graph *cfg.Graph, parent *scope.State) {
	t.Helper()
	if graph == nil || graph.ID() == 0 {
		t.Fatal("test graph has no ID")
	}
	if parent == nil || parent.Hash() == 0 {
		t.Fatal("test parent has no hash")
	}
	st.SetParentScope(parent.Hash(), parent)
	st.SetGraphParentHash(graph.ID(), parent.Hash())
}

func writeFunctionFactType(st *store.SessionStore, graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID, fnType typ.Type) {
	key := api.KeyForGraph(graph, parent.Hash())
	st.InterprocPrev.Facts[key] = api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Type: fnType},
		},
	}
}
