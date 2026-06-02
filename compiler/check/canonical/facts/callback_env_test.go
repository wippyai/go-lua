package facts

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallbackEnvConvertsOverlayNamesToSymbols(t *testing.T) {
	root := parseFactsTestChunk(t, `
run_cases(function()
    describe("suite", function()
        it("case", function()
        end)
    end)
end)
`)
	graphs := collectFactsTestGraphs(root, "run_cases", "describe", "it")
	refs := factsTestRefs(graphs)
	funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)
	rootGraph := graphs[root]
	callbackFn := rootGraph.NestedFunctions()[0].Func
	callbackGraph := graphs[callbackFn]
	nestedFn := callbackGraph.NestedFunctions()[0].Func
	nestedGraph := graphs[nestedFn]
	rootRef := ref.FuncRef{GraphID: rootGraph.ID()}
	callbackRef := ref.FuncRef{GraphID: callbackGraph.ID()}
	nestedRef := ref.FuncRef{GraphID: nestedGraph.ID()}

	runCases := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
			"describe": typ.Func().Returns(typ.Nil).Build(),
			"it":       typ.Func().Returns(typ.Nil).Build(),
		}))).
		Build()

	entries := collectCallbackEnv(Program{
		Refs: refs,
		Graph: func(r ref.FuncRef) *cfg.Graph {
			for _, g := range graphs {
				if g.ID() == r.GraphID {
					return g
				}
			}
			return nil
		},
		ResolveCallee: func(*cfg.Graph, *ast.FuncCallExpr) (ref.FuncRef, bool) {
			return ref.FuncRef{}, false
		},
		CalleeCallbackOverlays: func(_ *cfg.Graph, call *ast.FuncCallExpr) callbackenv.Overlays {
			if id, ok := call.Func.(*ast.IdentExpr); ok && id.Value == "run_cases" {
				return callbackenv.OverlaysFromFunction(runCases)
			}
			return nil
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
			r, ok := funcRefsBySymbol[sym]
			return r, ok
		},
		NestedFuncRefs: func(r ref.FuncRef) []ref.FuncRef {
			g := graphByRef(graphs, r)
			if g == nil {
				return nil
			}
			var out []ref.FuncRef
			for _, nested := range g.NestedFunctions() {
				if ng := graphs[nested.Func]; ng != nil {
					out = append(out, ref.FuncRef{GraphID: ng.ID()})
				}
			}
			return out
		},
	})

	if len(entries) == 0 {
		t.Fatal("expected callback env facts")
	}
	assertCallbackEnvBinding(t, entries, callbackRef, callbackGraph, "describe")
	assertCallbackEnvBinding(t, entries, callbackRef, callbackGraph, "it")
	assertCallbackEnvBinding(t, entries, nestedRef, nestedGraph, "describe")
	assertCallbackEnvBinding(t, entries, nestedRef, nestedGraph, "it")

	for _, entry := range entries {
		if entry.FuncRef == rootRef {
			t.Fatalf("root function received callback-only env fact: %+v", entry)
		}
		if entry.Binding.Symbol == 0 || entry.Binding.Type == nil {
			t.Fatalf("non-canonical callback env entry: %+v", entry)
		}
	}
}

func TestCallbackEnvJoinsDuplicateSymbolFacts(t *testing.T) {
	root := parseFactsTestChunk(t, `
local cb = function()
    ctx()
end
run_string(cb)
run_number(cb)
`)
	graphs := collectFactsTestGraphs(root, "run_string", "run_number")
	refs := factsTestRefs(graphs)
	rootGraph := graphs[root]
	callbackFn := rootGraph.NestedFunctions()[0].Func
	callbackGraph := graphs[callbackFn]
	callbackRef := ref.FuncRef{GraphID: callbackGraph.ID()}
	funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)

	runString := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
			"ctx": typ.String,
		}))).
		Build()
	runNumber := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(0, (&contract.CallbackSpec{}).WithEnvOverlay(map[string]typ.Type{
			"ctx": typ.Number,
		}))).
		Build()

	entries := collectCallbackEnv(Program{
		Refs: refs,
		Graph: func(r ref.FuncRef) *cfg.Graph {
			return graphByRef(graphs, r)
		},
		ResolveCallee: func(*cfg.Graph, *ast.FuncCallExpr) (ref.FuncRef, bool) {
			return ref.FuncRef{}, false
		},
		CalleeCallbackOverlays: func(_ *cfg.Graph, call *ast.FuncCallExpr) callbackenv.Overlays {
			id, ok := call.Func.(*ast.IdentExpr)
			if !ok {
				return nil
			}
			switch id.Value {
			case "run_string":
				return callbackenv.OverlaysFromFunction(runString)
			case "run_number":
				return callbackenv.OverlaysFromFunction(runNumber)
			default:
				return nil
			}
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
			r, ok := funcRefsBySymbol[sym]
			return r, ok
		},
	})

	ctxSym, ok := callbackGraph.GlobalSymbol("ctx")
	if !ok || ctxSym == 0 {
		t.Fatal("callback graph did not register ctx global")
	}
	var got typ.Type
	count := 0
	for _, entry := range entries {
		if entry.FuncRef == callbackRef && entry.Binding.Symbol == ctxSym {
			count++
			got = entry.Binding.Type
		}
	}
	if count != 1 {
		t.Fatalf("callback env should contain one joined ctx fact, got count=%d entries=%+v", count, entries)
	}
	want := value.JoinPrecise(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("joined ctx type = %v, want %v", got, want)
	}
}

func TestCallbackEnvOverlayFromMapNormalizesBoundaryMap(t *testing.T) {
	env := map[string]typ.Type{
		"z": typ.Number,
		"":  typ.String,
		"a": typ.Boolean,
		"n": nil,
	}

	overlay := callbackenv.OverlayFromContractMap(env)
	if len(overlay) != 2 {
		t.Fatalf("overlay len = %d, want 2: %+v", len(overlay), overlay)
	}
	if overlay[0].Name != callbackenv.GlobalName("a") || !typ.TypeEquals(overlay[0].Type, typ.Boolean) {
		t.Fatalf("overlay[0] = %+v, want a:boolean", overlay[0])
	}
	if overlay[1].Name != callbackenv.GlobalName("z") || !typ.TypeEquals(overlay[1].Type, typ.Number) {
		t.Fatalf("overlay[1] = %+v, want z:number", overlay[1])
	}

	env["a"] = typ.String
	if !typ.TypeEquals(overlay[0].Type, typ.Boolean) {
		t.Fatalf("overlay aliases mutable boundary map: got %v", overlay[0].Type)
	}
}

func parseFactsTestChunk(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	chunk, err := parse.Parse(strings.NewReader(src), "callback_env_test")
	if err != nil {
		t.Fatal(err)
	}
	return &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
}

func collectFactsTestGraphs(root *ast.FunctionExpr, globals ...string) map[*ast.FunctionExpr]*cfg.Graph {
	rootGraph := cfg.Build(root, globals...)
	graphs := map[*ast.FunctionExpr]*cfg.Graph{root: rootGraph}
	queue := []*cfg.Graph{rootGraph}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		for _, nested := range g.NestedFunctions() {
			if nested.Func == nil || graphs[nested.Func] != nil {
				continue
			}
			ng := cfg.BuildWithBindings(nested.Func, rootGraph.Bindings())
			graphs[nested.Func] = ng
			queue = append(queue, ng)
		}
	}
	return graphs
}

func factsTestRefs(graphs map[*ast.FunctionExpr]*cfg.Graph) []ref.FuncRef {
	refs := make([]ref.FuncRef, 0, len(graphs))
	for _, g := range graphs {
		refs = append(refs, ref.FuncRef{GraphID: g.ID()})
	}
	slices.SortFunc(refs, func(a, b ref.FuncRef) int {
		if c := cmp.Compare(a.GraphID, b.GraphID); c != 0 {
			return c
		}
		return cmp.Compare(a.ParentHash, b.ParentHash)
	})
	return refs
}

func factsTestFuncSymbolRefs(graphs map[*ast.FunctionExpr]*cfg.Graph) map[cfg.SymbolID]ref.FuncRef {
	out := make(map[cfg.SymbolID]ref.FuncRef)
	refFor := func(fn *ast.FunctionExpr) (ref.FuncRef, bool) {
		g := graphs[fn]
		if g == nil {
			return ref.FuncRef{}, false
		}
		return ref.FuncRef{GraphID: g.ID()}, true
	}
	for _, g := range graphs {
		for _, nested := range g.NestedFunctions() {
			if nested.Symbol == 0 || nested.Func == nil {
				continue
			}
			if r, ok := refFor(nested.Func); ok {
				out[nested.Symbol] = r
			}
		}
		g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
				return
			}
			if r, ok := refFor(info.FuncExpr); ok {
				out[info.Symbol] = r
			}
		})
		for _, lfa := range g.LocalFunctionAssignments() {
			if lfa.Symbol == 0 || lfa.Func == nil {
				continue
			}
			if r, ok := refFor(lfa.Func); ok {
				out[lfa.Symbol] = r
			}
		}
	}
	return out
}

func graphByRef(graphs map[*ast.FunctionExpr]*cfg.Graph, r ref.FuncRef) *cfg.Graph {
	for _, g := range graphs {
		if g.ID() == r.GraphID {
			return g
		}
	}
	return nil
}

func assertCallbackEnvBinding(t *testing.T, entries []callbackEnvRow, r ref.FuncRef, g *cfg.Graph, name string) {
	t.Helper()
	sym, ok := g.GlobalSymbol(name)
	if !ok || sym == 0 {
		t.Fatalf("global %q not registered in callback graph", name)
	}
	for _, entry := range entries {
		if entry.FuncRef == r && entry.Binding.Symbol == sym {
			return
		}
	}
	t.Fatalf("missing callback env fact for ref=%+v global=%q sym=%d in %+v", r, name, sym, entries)
}
