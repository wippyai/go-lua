package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestUnsealedLuaTypeChecksPublishNoSemanticFacts(t *testing.T) {
	stmts, bindings, built := parseUnsealedTypeChunk(t, `
local value = input
local tag = "string"
if type(value) == "string" then end
if type(value) == tag then end
`, "type")
	body := wirlower.Lower("unsealed-type", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	value := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	valuePath := path.NewPath(value, "value")
	var raw, calls, comparisons int
	for point := cfg.Point(0); int(point) < built.Graph.Size(); point++ {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op == wir.OpCall {
				calls++
			}
			if inst.Op == wir.OpBinOp && (inst.Operator == wir.BinEq || inst.Operator == wir.BinNe) {
				comparisons++
				if got := facts.BranchRefinements(point); len(got) != 0 {
					t.Fatalf("unsealed comparison refinements at %d: %#v", point, got)
				}
				if got := facts.BranchPathRelations(point); len(got) != 0 {
					t.Fatalf("unsealed type path relations at %d: %#v", point, got)
				}
				if got := facts.BranchPathEvidence(point); len(got) != 0 {
					t.Fatalf("unsealed type path evidence at %d: %#v", point, got)
				}
			}
		}
		body.ForEachBranchCheck(point, func(check wir.Check) bool {
			if check.Kind != wir.CheckTypeEqual && check.Kind != wir.CheckTypeNot {
				return true
			}
			raw++
			for _, refinement := range facts.BranchRefinements(point) {
				if refinement.TargetPath().Equal(valuePath) {
					t.Fatalf("unsealed type refinement at %d: %#v", point, refinement)
				}
			}
			if got := facts.BranchPathRelations(point); len(got) != 0 {
				t.Fatalf("unsealed type path relations at %d: %#v", point, got)
			}
			if got := facts.BranchPathEvidence(point); len(got) != 0 {
				t.Fatalf("unsealed type path evidence at %d: %#v", point, got)
			}
			return true
		})
	}
	if raw != 0 || calls != 2 || comparisons != 2 {
		t.Fatalf("unsealed WIR checks/calls/comparisons = %d/%d/%d, want 0/2/2", raw, calls, comparisons)
	}
}

func TestUnsealedLuaTypeChecksDoNotLeakThroughCompoundAssertOrReturnCondition(t *testing.T) {
	stmts, bindings, built := parseUnsealedTypeChunk(t, `
local ok = input
local value = input
if ok and type(value) == "string" then end
assert(type(value) == "string")
return type(value) == "string"
`, "type", "assert")
	body := wirlower.Lower("unsealed-type-composites", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	value := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 1), 0)
	valuePath := path.NewPath(value, "value")

	for point := cfg.Point(0); int(point) < built.Graph.Size(); point++ {
		for _, refinement := range facts.BranchRefinements(point) {
			if refinement.TargetPath().Equal(valuePath) {
				t.Fatalf("compound unsealed type refinement at %d: %#v", point, refinement)
			}
		}
		for _, post := range facts.PostconditionRefinements(point) {
			if post.TargetPath().Equal(valuePath) {
				t.Fatalf("assert published unsealed type postcondition at %d: %#v", point, post)
			}
		}
	}

	ret := stmts[len(stmts)-1].(*ast.ReturnStmt)
	points := built.StmtPoints.PointsFor(ret)
	if len(points) < 2 {
		t.Fatalf("return points = %v, want retained type call plus return", points)
	}
	point := points[len(points)-1]
	returnFact, ok := facts.Return(point)
	if !ok {
		t.Fatal("missing return fact")
	}
	for _, source := range returnFact.Sources() {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			continue
		}
		if condition, exists := facts.ExpressionCondition(source.ExprRef); exists && !condition.IsEmpty() {
			t.Fatalf("returned unsealed type predicate published expression condition: %#v", condition)
		}
	}
}

func parseUnsealedTypeChunk(t *testing.T, source string, globals ...string) ([]ast.Stmt, *bind.Result, *cfgbuild.Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "unsealed_type_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatal("BuildChunk returned nil")
	}
	return stmts, bindings, built
}

func TestSealedLuaTypeCheckPositiveControlPublishesFacts(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `local value = input if type(value) == "string" then end`, "type")
	body := wirlower.Lower("sealed-type", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body, SealedLuaTypeChecks: true}).Facts
	ifStmt := stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	if len(facts.BranchRefinements(point)) == 0 || len(facts.BranchPathEvidence(point)) == 0 {
		t.Fatalf("sealed positive control facts missing: refinements %#v evidence %#v", facts.BranchRefinements(point), facts.BranchPathEvidence(point))
	}
}
