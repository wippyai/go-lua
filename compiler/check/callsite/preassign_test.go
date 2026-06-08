package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPreAssignmentTargetsContainsAssignmentSourceCallTarget(t *testing.T) {
	graph := buildGraph(t, `
local x = "old"
x = update(x)
`)
	var assignPoint cfg.Point
	var assignInfo *cfg.AssignInfo
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info != nil && !info.IsLocal && len(info.SourceCalls) == 1 {
			assignPoint = p
			assignInfo = info
		}
	})
	if assignInfo == nil {
		t.Fatal("expected assignment with one source call")
	}
	targetSym := assignInfo.Targets[0].Symbol
	sourceCall := assignInfo.SourceCalls[0]

	targets := PreAssignmentTargetsByCall([]api.AssignmentEvidence{{Point: assignPoint, Info: assignInfo}})
	if !targets.Contains(sourceCall, targetSym) {
		t.Fatalf("pre-assignment targets did not contain call target symbol %d", targetSym)
	}
	if targets.Contains(sourceCall, cfg.SymbolID(99999)) {
		t.Fatal("pre-assignment targets contained unrelated symbol")
	}
}

func TestPreAssignmentTypeAtJoin_JoinsPredecessorsOnly(t *testing.T) {
	graph, joinPoint := buildGraphWithJoinPoint(t)
	sym := cfg.SymbolID(9001)
	preds := graph.Predecessors(joinPoint)

	byPred := make(map[cfg.Point]typ.Type, len(preds))
	var expected typ.Type
	for i, pred := range preds {
		var pt typ.Type
		if i%2 == 0 {
			pt = typ.String
		} else {
			pt = typ.Number
		}
		byPred[pred] = pt
		if expected == nil {
			expected = pt
		} else {
			expected = typ.JoinPreferNonSoft(expected, pt)
		}
	}

	got := PreAssignmentTypeAtJoin(graph, joinPoint, sym, func(p cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
		if id != sym {
			return nil, false
		}
		if t, ok := byPred[p]; ok {
			return t, true
		}
		// Point fallback must be ignored by predecessor-only join.
		if p == joinPoint {
			return typ.Boolean, true
		}
		return nil, false
	})

	if !typ.TypeEquals(got, expected) {
		t.Fatalf("pre-assignment join mismatch: got %v want %v", got, expected)
	}
}

func TestPreAssignmentTypeAtJoinOrPoint_FallsBackToPoint(t *testing.T) {
	graph, joinPoint := buildGraphWithJoinPoint(t)
	sym := cfg.SymbolID(9002)

	got := PreAssignmentTypeAtJoinOrPoint(graph, joinPoint, sym, func(p cfg.Point, id cfg.SymbolID) (typ.Type, bool) {
		if id != sym {
			return nil, false
		}
		if p == joinPoint {
			return typ.Integer, true
		}
		return nil, false
	})

	if !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("fallback type mismatch: got %v want %v", got, typ.Integer)
	}
}

func buildGraphWithJoinPoint(t *testing.T) (*cfg.Graph, cfg.Point) {
	t.Helper()

	code := `
if cond then
    local x = "a"
else
    local x = 1
end
return 0
`
	graph := buildGraph(t, code, "cond")

	var joinPoint cfg.Point
	for _, p := range graph.RPO() {
		if len(graph.Predecessors(p)) >= 2 {
			joinPoint = p
			break
		}
	}
	if joinPoint == 0 {
		t.Fatal("expected graph point with multiple predecessors")
	}
	return graph, joinPoint
}

func buildGraph(t *testing.T, code string, params ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "preassign.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: params},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, params...)
	if graph == nil {
		t.Fatal("expected graph")
	}
	return graph
}
