package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Constructor laws consume the exact Source and Flow owner rows.  They do not
// reconstruct a generic Program graph or a per-term recurrence identity.
func TestSourceNestedOperandsKeepAuthoredComposition(t *testing.T) {
	p := parseBindLower(t, "return not ((a()[b()][c()] + d()) and e(f(g()[h()])))")
	returned, ok := p.Flow().Authored().Control().Returns().At(0)
	if !ok {
		t.Fatal("missing Return")
	}
	_, values, returnOK := p.Flow().Authored().Control().Returns().Get(returned)
	if !returnOK {
		t.Fatal("invalid Return row")
	}
	outerUnary := valueAt(t, p, values, 0)
	_, unaryOp, selected, unaryOK := p.Flow().Authored().Operators().Unaries().Get(outerUnary)
	if !unaryOK || unaryOp != kind.UnaryNot || selected == 0 {
		t.Fatalf("outer Unary = op %v operand %v ok %v", unaryOp, selected, unaryOK)
	}
	_, selectOp, binary, outerCall, selectOK := p.Flow().Authored().Operators().Selects().Get(selected)
	if !selectOK || selectOp != kind.SelectAnd || binary == 0 || outerCall == 0 {
		t.Fatalf("Select = op %v left %v right %v ok %v", selectOp, binary, outerCall, selectOK)
	}
	_, binaryOp, outerRead, callD, binaryOK := p.Flow().Authored().Operators().Binaries().Get(binary)
	if !binaryOK || binaryOp != kind.BinaryAdd || outerRead == 0 || callD == 0 {
		t.Fatalf("Binary = op %v left %v right %v ok %v", binaryOp, outerRead, callD, binaryOK)
	}
	_, outerLens, _, readOK := p.Flow().Authored().Storage().Reads().Get(outerRead)
	if !readOK || outerLens == 0 {
		t.Fatalf("Binary left Read source = %v/%v", outerLens, readOK)
	}
	_, innerRead, callC, lensOK := p.Flow().Authored().Access().Dynamic().Get(outerLens)
	if !lensOK || innerRead == 0 || callC == 0 {
		t.Fatalf("outer dynamic Lens = base %v key %v ok %v", innerRead, callC, lensOK)
	}
	_, innerLens, _, innerReadOK := p.Flow().Authored().Storage().Reads().Get(innerRead)
	if !innerReadOK || innerLens == 0 {
		t.Fatalf("inner Read source = %v/%v", innerLens, innerReadOK)
	}
	_, callA, callB, innerLensOK := p.Flow().Authored().Access().Dynamic().Get(innerLens)
	if !innerLensOK || callA == 0 || callB == 0 {
		t.Fatalf("inner dynamic Lens = base %v key %v ok %v", callA, callB, innerLensOK)
	}
	for _, call := range []keyspace.Term{callA, callB, callC, callD, outerCall} {
		if _, _, _, _, ok := p.Flow().Authored().Calls().Get(call); !ok {
			t.Fatalf("nested operand %v is not an authored Call", call)
		}
	}
}

func nestedBranchSource(depth int) string {
	if depth == 0 {
		return "leaf()"
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("if condition")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("() then\n")
	}
	input.WriteString("leaf()\n")
	for index := depth - 1; index >= 0; index-- {
		input.WriteString("else\nfallthrough")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("()\nend\n")
	}
	return input.String()
}

func TestSourceNestedBranchesKeepSourceParents(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedBranchSource(depth))
			entry, ok := p.Source().Index().Entry()
			if !ok {
				t.Fatal("missing entry Body")
			}
			if got := p.Flow().Authored().Control().Branches().Count(); got != depth {
				t.Fatalf("Branch count = %d, want %d", got, depth)
			}
			body := entry
			for level := 0; level < depth; level++ {
				branch := controlSourceAt(t, p, body, 0)
				owner, condition, whenTrue, whenFalse, branchOK := p.Flow().Authored().Control().Branches().Get(branch)
				if !branchOK || owner != body || condition == 0 || whenTrue == 0 || whenFalse == 0 {
					t.Fatalf("Branch depth %d = owner %v condition %v true %v false %v ok %v", level, owner, condition, whenTrue, whenFalse, branchOK)
				}
				if parent, ok := p.Source().Index().BodyParent(whenTrue); !ok || parent != body {
					t.Fatalf("truthy Body parent at depth %d = %v/%v, want %v", level, parent, ok, body)
				}
				if parent, ok := p.Source().Index().BodyParent(whenFalse); !ok || parent != body {
					t.Fatalf("false Body parent at depth %d = %v/%v, want %v", level, parent, ok, body)
				}
				body = whenTrue
			}
		})
	}
}

func nestedWhileSource(depth int) string {
	if depth == 0 {
		return "step()"
	}
	var input strings.Builder
	for index := 0; index < depth; index++ {
		input.WriteString("while test")
		input.WriteString(strconv.Itoa(index))
		input.WriteString("() do\n")
	}
	input.WriteString("step()\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	return input.String()
}

func TestSourceNestedLoopsKeepCausalResetSupport(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedWhileSource(depth))
			loops := p.Flow().Authored().Control().Loops()
			if loops.Count() != depth {
				t.Fatalf("Loop count = %d, want %d", loops.Count(), depth)
			}
			edges := p.Flow().Causal().Edges()
			for index := 0; index < loops.Count(); index++ {
				loop, _ := loops.At(index)
				_, body, loopKind, _, ok := loops.Get(loop)
				if !ok || body == 0 || loopKind != kind.LoopWhile {
					t.Fatalf("Loop[%d] = body %v kind %v ok %v", index, body, loopKind, ok)
				}
				if _, ok := p.Flow().Outcomes().BodyExit(body, kind.OutcomeNormal); !ok {
					t.Fatalf("Loop[%d] Body has no normal Outcome", index)
				}
				found := false
				for edgeIndex := 0; edgeIndex < edges.Count(); edgeIndex++ {
					found = found || edges.ResetContains(edgeIndex, loop)
				}
				if !found {
					t.Fatalf("Loop[%d] has no Causal reset support", index)
				}
			}
		})
	}
}

func nestedGotoSource(depth int) string {
	var input strings.Builder
	input.WriteString("::head::\n")
	for index := 0; index < depth; index++ {
		input.WriteString("do\n")
	}
	input.WriteString("goto head\n")
	for index := 0; index < depth; index++ {
		input.WriteString("end\n")
	}
	return input.String()
}

func TestSourceNestedGotosKeepExactTargetAndRecurrenceEdge(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 37} {
		t.Run("depth_"+strconv.Itoa(depth), func(t *testing.T) {
			p := parseBindLower(t, nestedGotoSource(depth))
			label, labelOK := p.Flow().Authored().Control().Labels().At(0)
			jump, jumpOK := p.Flow().Authored().Control().Gotos().At(0)
			if !labelOK || !jumpOK {
				t.Fatalf("Label/Goto = %v/%v %v/%v", label, labelOK, jump, jumpOK)
			}
			if _, target, ok := p.Flow().Authored().Control().Gotos().Get(jump); !ok || target != label {
				t.Fatalf("Goto target = %v/%v, want %v", target, ok, label)
			}
			if exit, ok := p.Flow().Outcomes().GotoExit(jump); !ok || exit == 0 {
				t.Fatalf("Goto exit = %v/%v", exit, ok)
			}
			found := false
			edges := p.Flow().Causal().Edges()
			for index := 0; index < edges.Count(); index++ {
				edge, edgeOK := edges.At(index)
				found = found || edgeOK && edge.Mu == label
			}
			if !found {
				t.Fatal("backward Goto has no Causal recurrence Edge")
			}
		})
	}
}

func TestSourceNestedFunctionsUseFormalsAndCaptureRows(t *testing.T) {
	p := parseBindLower(t, "\nlocal root = 0\nlocal function outer(first, second)\n  local rootDirect = root\n  local function middle(third)\n    local firstDirect = first\n    local function inner(fourth)\n      return third\n    end\n    return inner\n  end\n  return middle\nend\nreturn outer\n")
	functions := p.Flow().Authored().Functions()
	outer, _ := functions.At(0)
	middle, _ := functions.At(1)
	inner, _ := functions.At(2)
	outerOwner, outerBody, _, outerOK := functions.Get(outer)
	middleOwner, middleBody, _, middleOK := functions.Get(middle)
	innerOwner, innerBody, _, innerOK := functions.Get(inner)
	if !outerOK || !middleOK || !innerOK || outerOwner == 0 || middleOwner != outerBody || innerOwner != middleBody {
		t.Fatalf("Function owners outer=%v/%v middle=%v/%v inner=%v/%v", outerOwner, outerOK, middleOwner, middleOK, innerOwner, innerOK)
	}
	for _, row := range []struct {
		function keyspace.Term
		body     keyspace.Term
		count    int
	}{
		{outer, outerBody, 2}, {middle, middleBody, 1}, {inner, innerBody, 1},
	} {
		if count, ok := p.Source().Formals().Len(row.function); !ok || count != row.count {
			t.Fatalf("Formal count = %d/%v, want %d", count, ok, row.count)
		}
		for index := 0; index < row.count; index++ {
			formal, ok := p.Source().Formals().At(row.function, index)
			if !ok {
				t.Fatalf("missing Formal[%d]", index)
			}
			cellKind, host, _, cellOK := p.Flow().Authored().Storage().Cells().Get(formal)
			if !cellOK || cellKind != flow.CellLocal || host != row.body {
				t.Fatalf("Formal Cell = kind %v host %v ok %v, want local/%v", cellKind, host, cellOK, row.body)
			}
		}
	}
	outerFirst, _ := p.Source().Formals().At(outer, 0)
	middleThird, _ := p.Source().Formals().At(middle, 0)
	_, outerCaptured, outerCaptureOK := functions.CaptureAt(outer, 0)
	_, middleCaptured, middleCaptureOK := functions.CaptureAt(middle, 0)
	_, innerCaptured, innerCaptureOK := functions.CaptureAt(inner, 0)
	if !outerCaptureOK || !middleCaptureOK || !innerCaptureOK || outerCaptured == 0 || middleCaptured != outerFirst || innerCaptured != middleThird {
		t.Fatalf("Capture outers = %v/%v %v/%v %v/%v", outerCaptured, outerCaptureOK, middleCaptured, middleCaptureOK, innerCaptured, innerCaptureOK)
	}
}
