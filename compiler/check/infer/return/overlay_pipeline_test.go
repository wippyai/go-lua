package infer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestScratchCallablesUsesSummariesAndOwnerDiscriminantsOnly(t *testing.T) {
	stmts, err := parse.ParseString(`
local summarized = function() end
local plain = function() end
local overloaded = function() end
`, "test.lua")
	if err != nil {
		t.Fatal(err)
	}
	graph := cfg.Build(&ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts})
	functions := make(map[string]*ast.FunctionExpr)
	var summarySym cfg.SymbolID
	graph.EachLocalFunction(func(_ cfg.Point, sym cfg.SymbolID, fn *ast.FunctionExpr) {
		name := graph.NameOf(sym)
		functions[name] = fn
		if name == "summarized" {
			summarySym = sym
		}
	})
	if len(functions) != 3 || summarySym == 0 {
		t.Fatalf("unexpected local functions: %v", functions)
	}

	general := typ.Func().Returns(typ.String).Build()
	overload := typ.NewIntersection(typ.Func().Returns(typ.Number).Build(), general)
	parent := scope.New()
	facts := api.Callables{
		functions["summarized"]: {Summary: []typ.Type{typ.Number}, Narrow: []typ.Type{typ.Number}, Func: general},
		functions["plain"]:      {Summary: []typ.Type{typ.Number}, Narrow: []typ.Type{typ.Number}, Func: general},
		functions["overloaded"]: {Summary: []typ.Type{typ.Number}, Narrow: []typ.Type{typ.Number}, Func: overload},
	}
	s := store.NewSessionStore()
	s.InterprocPrev.Facts[api.KeyForGraph(graph, parent.Hash())] = api.Facts{Callables: facts}
	ctx := &returnInferenceContext{
		info:      &returns.LocalFuncInfo{Graph: graph, DefScope: parent},
		summaries: map[cfg.SymbolID][]typ.Type{summarySym: {typ.String}},
	}
	got := New(Config{Store: s}).scratchCallables(ctx)
	if len(got) != 2 {
		t.Fatalf("expected summary and overload only, got %v", got)
	}
	if fact := got[functions["summarized"]]; len(fact.Summary) != 1 || fact.Summary[0] != typ.String || len(fact.Narrow) != 1 || fact.Narrow[0] != typ.String || fact.Func != nil {
		t.Fatalf("summary must supply both vectors without plain owner type: %+v", fact)
	}
	if _, ok := got[functions["plain"]]; ok {
		t.Fatal("plain owner fact must not enter scratch without a current summary")
	}
	if fact := got[functions["overloaded"]]; fact.Func != overload || len(fact.Summary) != 0 || len(fact.Narrow) != 0 {
		t.Fatalf("overload must contribute only its discriminants: %+v", fact)
	}
}
