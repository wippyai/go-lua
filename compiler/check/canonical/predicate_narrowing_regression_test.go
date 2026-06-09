package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalLocalPredicateTrueBranchNarrowsAssignmentSource(t *testing.T) {
	src := `
local function is_positive_number(value)
	return type(value) == "number" and value > 0
end

local function run(value: any)
	if is_positive_number(value) then
		local narrowed: number = value
		return narrowed + 1
	end
	return 0
end

return run
`
	res := testutil.Check(src, testutil.WithStdlib())
	fn := findFunctionWithLocalTarget(t, res.Session.Results, "narrowed")
	valueSym := paramSymbolInGraphNamed(t, fn.Graph, "value")
	point, targetSym, source := assignmentSourceForTarget(t, fn.Graph, "narrowed")
	valuePath := constraint.NewPath(valueSym, "value")

	if got := fn.NarrowedTypeAt(point, valuePath); !typ.TypeEquals(got, typ.Number) {
		cond := conditionAt(t, fn.TypeFacts(), point)
		t.Fatalf("NarrowedTypeAt(value at narrowed assignment) = %v, want number; cond=%v; diagnostics=%v", got, cond, testutil.ErrorMessages(res.Diagnostics))
	}
	if got := observation.FromFuncResult(fn, nil).WithProofValues().AssignmentSourceType(source, point, typ.Number, targetSym); !typ.TypeEquals(got, typ.Number) {
		cond := conditionAt(t, fn.TypeFacts(), point)
		t.Fatalf("AssignmentSourceType(value at narrowed assignment) = %v, want number; cond=%v; diagnostics=%v", got, cond, testutil.ErrorMessages(res.Diagnostics))
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", diagnosticStrings(res.Diagnostics))
	}
}

func diagnosticStrings(diags []diag.Diagnostic) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.String())
	}
	return out
}

func findFunctionWithLocalTarget(t *testing.T, results map[*ast.FunctionExpr]*api.FuncResult, name string) *api.FuncResult {
	t.Helper()
	for _, result := range results {
		if result == nil || result.Graph == nil {
			continue
		}
		found := false
		result.Graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil {
				return
			}
			for _, target := range info.Targets {
				if target.Kind == cfg.TargetIdent && target.Name == name {
					found = true
					return
				}
			}
		})
		if found {
			return result
		}
	}
	t.Fatalf("no function with local target %q", name)
	return nil
}

func paramSymbolInGraphNamed(t *testing.T, g *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	for _, sym := range g.ParamSymbols() {
		if g.NameOf(sym) == name {
			return sym
		}
	}
	t.Fatalf("no param %q", name)
	return 0
}
