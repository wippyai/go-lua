package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalDynamicIndexEqualityNarrowsBaseUnionThroughConditionProof(t *testing.T) {
	src := `
type A = {[string]: "a"}
type B = {[string]: "b"}

function f(t: A | B, k: string)
    if t[k] == "a" then
        local x: A = t
    else
        local y: B = t
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "t", "k")
	tSym := singleSymbolNamed(t, fn.Graph, "t")
	tPath := constraint.NewPath(tSym, "t")
	aType := typ.NewMap(typ.String, typ.LiteralString("a"))
	bType := typ.NewMap(typ.String, typ.LiteralString("b"))
	obs := observation.FromFuncResult(fn, nil).WithProofValues()

	for name, want := range map[string]typ.Type{"x": aType, "y": bType} {
		point, targetSym, source := assignmentSourceForTarget(t, fn.Graph, name)
		if got := conditionTypeAt(t, fn.Facts, point, tPath); !typ.TypeEquals(got, want) {
			cond := conditionAt(t, fn.Facts, point)
			t.Fatalf("ConditionTypeAt(t at %s assignment) = %v, want %v; cond=%v; diagnostics=%v", name, got, want, cond, testutil.ErrorMessages(res.Diagnostics))
		}
		if got := obs.AssignmentSourceType(source, point, want, targetSym); !typ.TypeEquals(got, want) {
			cond := conditionAt(t, fn.Facts, point)
			t.Fatalf("AssignmentSourceType(t at %s assignment) = %v, want %v; cond=%v; diagnostics=%v", name, got, want, cond, testutil.ErrorMessages(res.Diagnostics))
		}
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
}

func TestCanonicalConstEnumInequalityEarlyReturnUsesConditionProof(t *testing.T) {
	src := `
type ExitEvent = {kind: "exit", code: number}
type CancelEvent = {kind: "cancel"}
type Event = ExitEvent | CancelEvent

local EXIT = "exit"

function handle(event: Event): number
    if event.kind ~= EXIT then
        return 0
    end
    return event.code
end
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithParamNames(t, res.Session.Results, "event")
	eventSym := singleSymbolNamed(t, fn.Graph, "event")
	returnPoint := lastReturnPoint(t, fn.Graph)
	codePath := constraint.NewPath(eventSym, "event").Field("code")
	if got := conditionTypeAt(t, fn.Facts, returnPoint, codePath); !typ.TypeEquals(got, typ.Number) {
		cond := conditionAt(t, fn.Facts, returnPoint)
		t.Fatalf("ConditionTypeAt(event.code at final return) = %v, want number; cond=%v; diagnostics=%v", got, cond, testutil.ErrorMessages(res.Diagnostics))
	}
	returnExpr := lastReturnExpr(t, fn.Graph)
	if got := observation.FromFuncResult(fn, nil).WithProofValues().ReturnSourceType(returnExpr, returnPoint, typ.Number); !typ.TypeEquals(got, typ.Number) {
		cond := conditionAt(t, fn.Facts, returnPoint)
		t.Fatalf("ReturnSourceType(event.code at final return) = %v, want number; cond=%v; diagnostics=%v", got, cond, testutil.ErrorMessages(res.Diagnostics))
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
}

func conditionAt(t *testing.T, facts interface{}, point cfg.Point) constraint.Condition {
	t.Helper()
	cf, ok := facts.(interface {
		ConditionAt(cfg.Point) constraint.Condition
	})
	if !ok {
		t.Fatal("canonical facts do not expose conditions")
	}
	return cf.ConditionAt(point)
}

func conditionTypeAt(t *testing.T, facts interface{}, point cfg.Point, path constraint.Path) typ.Type {
	t.Helper()
	cf, ok := facts.(interface {
		ConditionTypeAt(cfg.Point, constraint.Path) typ.Type
	})
	if !ok {
		t.Fatal("canonical facts do not expose condition type proofs")
	}
	return cf.ConditionTypeAt(point, path)
}

func lastReturnPoint(t *testing.T, g *cfg.Graph) cfg.Point {
	t.Helper()
	var point cfg.Point
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		point = p
	})
	if point == 0 {
		t.Fatal("no return point")
	}
	return point
}

func lastReturnExpr(t *testing.T, g *cfg.Graph) ast.Expr {
	t.Helper()
	var expr ast.Expr
	g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		expr = info.Exprs[0]
	})
	if expr == nil {
		t.Fatal("no return expression")
	}
	return expr
}
