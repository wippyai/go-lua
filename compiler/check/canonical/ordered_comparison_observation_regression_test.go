package canonical_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalOrderedComparisonConditionProofFeedsAssignmentObservation(t *testing.T) {
	src := `
local function classify(value)
	if value < "m" then
		local s: string = value
		return s
	end
	local s: string = value
	return s
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "value")
	valueSym := paramSymbolInGraphNamed(t, fn.Graph, "value")
	valuePath := constraint.NewPath(valueSym, "value")
	obs := observation.FromFuncResult(fn, nil).WithProofValues()

	for _, name := range []string{"s"} {
		point, targetSym, source := assignmentSourceForTarget(t, fn.Graph, name)
		if got := conditionTypeAt(t, fn.Facts, point, valuePath); !typ.TypeEquals(got, typ.String) {
			cond := conditionAt(t, fn.Facts, point)
			t.Fatalf("ConditionTypeAt(value at %s assignment) = %v, want string; cond=%v; diagnostics=%v", name, got, cond, testutil.ErrorMessages(res.Diagnostics))
		}
		if got := obs.AssignmentSourceType(source, point, typ.String, targetSym); !typ.TypeEquals(got, typ.String) {
			cond := conditionAt(t, fn.Facts, point)
			t.Fatalf("AssignmentSourceType(value at %s assignment) = %v, want string; cond=%v; diagnostics=%v", name, got, cond, testutil.ErrorMessages(res.Diagnostics))
		}
	}
}

func TestCanonicalSelfReferentialTableLiteralUsesNarrowedAssignmentSource(t *testing.T) {
	src := `
type Entry = string | { id: string }

local function wrap(entry: Entry | Entry[])
	if type(entry) == "string" or (type(entry) == "table" and entry.id) then
		entry = { entry }
	end
	return entry
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "entry")
	targetSym := paramSymbolInGraphNamed(t, fn.Graph, "entry")
	var point cfg.Point
	var table *ast.TableExpr
	fn.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if point != 0 || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, _ cfg.AssignTarget, src ast.Expr) {
			if point != 0 {
				return
			}
			if tbl, ok := src.(*ast.TableExpr); ok && tbl != nil {
				point = p
				table = tbl
			}
		})
	})
	if point == 0 || table == nil {
		t.Fatal("no table literal assignment source")
	}
	obs := observation.FromFuncResult(fn, nil).WithProofValues()
	expected := fn.Facts.DeclaredAt(point, targetSym).Type
	if typ.IsAbsentOrUnknown(expected) {
		t.Fatal("entry assignment has no declared target type")
	}
	if result := obs.AssignmentSourceTableCheck(table, point, expected, targetSym); !result.Handled || !result.Compatible {
		cond := conditionAt(t, fn.Facts, point)
		refined := fn.Facts.RefinedAt(point, targetSym)
		var elemType typ.Type
		if len(table.Fields) > 0 && table.Fields[0] != nil {
			elemType = obs.AssignmentSourceType(table.Fields[0].Value, point, nil, targetSym)
		}
		t.Fatalf("AssignmentSourceTableCheck(entry rewrap) = %#v, want compatible; point=%d target=%d refined=%v; elem=%v; cond=%v; preds=%s; branches=%s; diagnostics=%v",
			result, point, targetSym, refined.Type, elemType, cond, predecessorStateSummary(fn, point, targetSym), branchSummary(fn), testutil.ErrorMessages(res.Diagnostics))
	}
}

func predecessorStateSummary(fn *api.FuncResult, point cfg.Point, sym cfg.SymbolID) string {
	if fn == nil || fn.Graph == nil || fn.Facts == nil {
		return ""
	}
	var parts []string
	postFacts, _ := fn.Facts.(interface {
		PostEffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue
	})
	for _, pred := range fn.Graph.Predecessors(point) {
		tv := fn.Facts.RefinedAt(pred, sym)
		var post flow.TypedValue
		if postFacts != nil {
			post = postFacts.PostEffectiveTypeAt(pred, sym)
		}
		parts = append(parts, fmt.Sprintf("%d:in=%v out=%v cond=%v ppreds=%v", pred, tv.Type, post.Type, conditionAtNoFatal(fn.Facts, pred), fn.Graph.Predecessors(pred)))
	}
	return strings.Join(parts, "; ")
}

func branchSummary(fn *api.FuncResult) string {
	if fn == nil || fn.Graph == nil {
		return ""
	}
	target := cfg.SymbolID(0)
	for _, sym := range fn.Graph.ParamSymbols() {
		if fn.Graph.NameOf(sym) == "entry" {
			target = sym
			break
		}
	}
	postFacts, _ := fn.Facts.(interface {
		PostEffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue
	})
	var parts []string
	fn.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		in := fn.Facts.RefinedAt(p, target)
		var post flow.TypedValue
		if postFacts != nil {
			post = postFacts.PostEffectiveTypeAt(p, target)
		}
		var succs []string
		for _, succ := range fn.Graph.Successors(p) {
			taken, ok := fn.Graph.EdgeCond(p, succ)
			succs = append(succs, fmt.Sprintf("%d:%v/%v", succ, taken, ok))
		}
		parts = append(parts, fmt.Sprintf("%d:%T check=%v type=%q sym=%d var=%q in=%v out=%v succ=%v", p, info.Condition, info.CondCheck.Kind, info.CondCheck.TypeName, info.CondSymbol, info.CondVar, in.Type, post.Type, strings.Join(succs, ",")))
	})
	return strings.Join(parts, "; ")
}

func conditionAtNoFatal(facts interface{}, point cfg.Point) constraint.Condition {
	cf, ok := facts.(interface {
		ConditionAt(cfg.Point) constraint.Condition
	})
	if !ok {
		return constraint.TrueCondition()
	}
	return cf.ConditionAt(point)
}
