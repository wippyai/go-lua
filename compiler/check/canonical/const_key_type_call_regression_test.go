package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalTypeCallConstKeyPathSurvivesToAssignmentObservation(t *testing.T) {
	src := `
type Point = {x: number, y: number}
function validate(obj: {["p-q"]: any})
    local key = "p-q"
    Point(obj[key])
    local p: {x: number, y: number} = obj[key]
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	fn := findFunctionWithParamNames(t, res.Session.Results, "obj")
	objSym := singleSymbolNamed(t, fn.Graph, "obj")
	assignPoint, targetSym, source := assignmentSourceForTarget(t, fn.Graph, "p")
	pointType := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	path := constraint.NewPath(objSym, "obj").IndexStr("p-q")

	pathFacts, ok := fn.Facts.(flow.PathFacts)
	if !ok {
		t.Fatal("canonical facts do not expose path facts")
	}
	tv := pathFacts.RefinedPathAt(assignPoint, path)
	if tv.State != flow.StateResolved || typ.IsAbsentOrUnknown(tv.Type) || typ.IsAny(tv.Type) {
		t.Fatalf("RefinedPathAt(obj[\"p-q\"] at assignment) = %v/%v, want non-any Point proof; diagnostics=%v", tv.Type, tv.State, testutil.ErrorMessages(res.Diagnostics))
	}

	got := observation.FromFuncResult(fn, nil).WithProofValues().AssignmentSourceType(source, assignPoint, pointType, targetSym)
	if !typ.TypeEquals(got, tv.Type) {
		t.Fatalf("AssignmentSourceType(obj[key]) = %v, want normalized path proof %v; diagnostics=%v", got, tv.Type, testutil.ErrorMessages(res.Diagnostics))
	}
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check after const-key path proof, got diagnostics: %v", msgs)
	}
}

func assignmentSourceForTarget(t *testing.T, g *cfg.Graph, name string) (cfg.Point, cfg.SymbolID, ast.Expr) {
	t.Helper()
	var point cfg.Point
	var sym cfg.SymbolID
	var source ast.Expr
	g.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if point != 0 || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if point != 0 || target.Name != name {
				return
			}
			point = p
			sym = target.Symbol
			source = src
		})
	})
	if point == 0 || sym == 0 || source == nil {
		t.Fatalf("no assignment source for target %q", name)
	}
	return point, sym, source
}
