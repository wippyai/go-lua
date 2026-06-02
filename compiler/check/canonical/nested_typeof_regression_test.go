package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalNestedTypeofElseAssignmentObservationUsesConditionAxis(t *testing.T) {
	src := `
function f(x: string | number | boolean)
    if type(x) == "string" then
        local s: string = x
    elseif type(x) == "number" then
        local n: number = x
    else
        local b: boolean = x
    end
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	fn := findFunctionWithParamNames(t, res.Session.Results, "x")
	obs := observation.FromFuncResult(fn, nil).WithProofValues()

	for name, want := range map[string]typ.Type{
		"s": typ.String,
		"n": typ.Number,
		"b": typ.Boolean,
	} {
		point, targetSym, source := assignmentSourceForTarget(t, fn.Graph, name)
		got := obs.AssignmentSourceType(source, point, want, targetSym)
		if !typ.TypeEquals(got, want) {
			var cond constraint.Condition
			if facts, ok := fn.Facts.(interface {
				ConditionAt(cfg.Point) constraint.Condition
			}); ok {
				cond = facts.ConditionAt(point)
			}
			t.Fatalf("AssignmentSourceType(x at %s assignment) = %v, want %v; cond=%v; diagnostics=%v", name, got, want, cond, testutil.ErrorMessages(res.Diagnostics))
		}
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check, got diagnostics: %v", msgs)
	}
}
