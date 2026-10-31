package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNew(t *testing.T) {
	d := New(Config{
		MaxIterations: 5,
		MaxScopeDepth: 10,
	})
	if d == nil {
		t.Fatal("expected non-nil driver")
	}
	if d.cfg.MaxIterations != 5 {
		t.Error("MaxIterations not set")
	}
	if d.cfg.MaxScopeDepth != 10 {
		t.Error("MaxScopeDepth not set")
	}
}

func TestDriver_Run_NilSession(t *testing.T) {
	d := New(Config{})
	d.Run(nil, nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
		MaxScopeDepth: 8,
		EmitScopeDiag: true,
		GlobalTypes:   map[string]typ.Type{"foo": typ.String},
	}
	if cfg.MaxIterations != 3 {
		t.Error("MaxIterations not set")
	}
	if cfg.MaxScopeDepth != 8 {
		t.Error("MaxScopeDepth not set")
	}
	if !cfg.EmitScopeDiag {
		t.Error("EmitScopeDiag not set")
	}
	if cfg.GlobalTypes["foo"] != typ.String {
		t.Error("GlobalTypes not set")
	}
}

func TestCollectGlobalNames(t *testing.T) {
	globals := map[string]typ.Type{
		"print": typ.Any,
		"error": typ.Any,
	}
	names := collectGlobalNames(globals)
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["print"] {
		t.Error("print not found")
	}
	if !found["error"] {
		t.Error("error not found")
	}
}

type factsMarker struct {
	flow.TypeFacts
	name string
}

type resultsOnlySession struct {
	api.AnalysisSession
	results map[*ast.FunctionExpr]*api.FuncResult
}

func (s resultsOnlySession) ResultsMap() map[*ast.FunctionExpr]*api.FuncResult {
	return s.results
}

// Return inference for a graph infers the local functions defined in it; their
// definition points are points of that graph, so the facts it reads are the
// graph's own solved facts, not those of the graph's parent.
func TestLocalFunctionFactsAreTheDefiningGraphs(t *testing.T) {
	stmts, err := parse.ParseString(`
local function outer()
	local function inner() return 1 end
	return inner()
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts}
	outer := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)

	rootGraph := cfg.Build(root)
	outerGraph := cfg.Build(outer)
	st := store.NewSessionStore()
	st.RegisterGraph(rootGraph, root)
	st.RegisterGraph(outerGraph, outer)
	st.RegisterNestedMeta(outerGraph.ID(), rootGraph.ID(), 1)

	rootFacts := factsMarker{name: "root"}
	outerFacts := factsMarker{name: "outer"}
	sess := resultsOnlySession{results: map[*ast.FunctionExpr]*api.FuncResult{
		root:  {Facts: rootFacts},
		outer: {Facts: outerFacts},
	}}

	got, ok := New(Config{}).localFunctionFacts(sess, st, outerGraph.ID()).(factsMarker)
	if !ok || got.name != "outer" {
		t.Fatalf("local functions of outer read the facts of %q, want outer", got.name)
	}
}
