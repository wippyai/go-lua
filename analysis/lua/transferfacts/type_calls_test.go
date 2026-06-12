package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerTypeCastCallPublishesArgumentAndResultEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local v = Point(data)
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	dataStmt := mustLocalStmt(t, stmts, 1)
	castStmt := mustLocalStmt(t, stmts, 2)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	refinements := facts.PostconditionRefinements(callPoint)
	if len(refinements) != 1 {
		t.Fatalf("postcondition refinements = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(dataPath) {
		t.Fatalf("postcondition target = %s, want %s", refinements[0].TargetPath(), dataPath)
	}
	assertProductPointLike(t, reg, refinementConstraint(t, refinements[0].Value()))

	results := facts.CallResultValues(callPoint)
	if len(results) != 1 {
		t.Fatalf("call result values = %d, want 1: %#v", len(results), results)
	}
	if results[0].Index() != 0 {
		t.Fatalf("call result index = %d, want 0", results[0].Index())
	}
	assertProductPointLike(t, reg, results[0].Value())
}

func TestLowerTypeIsErrorNilBranchPublishesArgumentEvidence(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local data: any = {}
local _, err = Point:is(data)
if err == nil then
end
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	dataStmt := mustLocalStmt(t, stmts, 1)
	ifStmt := mustIfStmt(t, stmts, 3)
	dataPath := path.NewPath(mustLocalAt(t, bindings, dataStmt, 0), "data")
	branchPoint := requireStmtPoints(t, built, ifStmt, 1)[0]

	var found bool
	for _, refinement := range facts.BranchRefinements(branchPoint) {
		if !refinement.TargetPath().Equal(dataPath) {
			continue
		}
		value, ok := refinement.TrueValue()
		if !ok {
			continue
		}
		assertProductPointLike(t, reg, refinementConstraint(t, value))
		found = true
	}
	if !found {
		t.Fatalf("missing true-edge Point refinement for %s at branch %d", dataPath, branchPoint)
	}
}

func TestLowerTypeCastCallIgnoresValueShadow(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}
local Point = function(value) return value end
local data: any = {}
local v = Point(data)
`)
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings})
	castStmt := mustLocalStmt(t, stmts, 3)
	castCall := castStmt.Exprs[0].(*ast.FuncCallExpr)
	callPoint := requireCallPoint(t, built.Graph, result, castCall)

	if refinements := facts.PostconditionRefinements(callPoint); len(refinements) != 0 {
		t.Fatalf("shadowed value call postconditions = %#v, want none", refinements)
	}
	if results := facts.CallResultValues(callPoint); len(results) != 0 {
		t.Fatalf("shadowed value call results = %#v, want none", results)
	}
}

func requireCallPoint(t *testing.T, graph cfg.Graph, result *semantics.Result, call *ast.FuncCallExpr) cfg.Point {
	t.Helper()
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if ok && fact.Call == call {
			return point
		}
	}
	t.Fatalf("missing call point for %#v", call)
	return 0
}

func refinementConstraint(t *testing.T, refinement factflow.ValueRefinement) product.Value {
	t.Helper()
	value, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("missing refinement constraint")
	}
	return value
}

func assertProductPointLike(t *testing.T, reg *axis.Registry, value product.Value) {
	t.Helper()
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("presence = %s, want present", got)
	}
	if got := product.Get(reg, value, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtime kind = %s, want table", got)
	}
}
