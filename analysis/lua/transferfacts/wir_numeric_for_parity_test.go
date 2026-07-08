package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerWithWIRNumericForProofsPublishForBothLoopDirections(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
	for j = #xs, 1, -1 do
		local current = xs[j]
	end
end
`)
	body := wirlower.LowerFunction("scan", fn, bindings, built)
	wirFacts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})

	compared := 0
	for _, point := range built.Graph.RPO() {
		if !body.HasInstruction(point, wir.OpIterate) {
			continue
		}
		compared++
		if got := wirFacts.BranchNumFloorRefinements(point); len(got) == 0 {
			t.Fatalf("missing WIR numeric-for num-floor proof at point %d", point)
		}
		if got := wirFacts.BranchPathEvidence(point); len(got) == 0 {
			t.Fatalf("missing WIR numeric-for path evidence at point %d", point)
		}
	}
	if compared != 2 {
		t.Fatalf("checked %d WIR numeric-for iterate points, want two loops", compared)
	}
}

func TestLowerWithWIRNumericForProofsDoesNotFallbackToSidecar(t *testing.T) {
	fn, _, built := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
end
`)
	loop := requireNumericForStmt(t, fn, 0)
	checkPoint := requireBranchPointForStmt(t, built, loop)
	facts := Lower(built.Graph, Config{
		Registry: standard.Registry(),
		WIR:      wir.NewBody("scan"),
	})

	if got := facts.BranchNumFloorRefinements(checkPoint); len(got) != 0 {
		t.Fatalf("WIR numeric-for num-floor proofs fell back to sidecar at point %d: %#v", checkPoint, got)
	}
	if got := facts.BranchPathEvidence(checkPoint); len(got) != 0 {
		t.Fatalf("WIR numeric-for path evidence fell back to sidecar at point %d: %#v", checkPoint, got)
	}
}

func TestLowerWithWIRNumericForProofsPublishWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
end
`)
	body := wirlower.LowerFunction("numeric-for-no-sidecars", fn, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})

	var checked int
	for _, point := range built.Graph.RPO() {
		if !body.HasInstruction(point, wir.OpIterate) {
			continue
		}
		checked++
		if got := facts.BranchNumFloorRefinements(point); len(got) == 0 {
			t.Fatalf("missing WIR numeric-for num-floor proof at point %d without semantic sidecars", point)
		}
		if got := facts.BranchPathEvidence(point); len(got) == 0 {
			t.Fatalf("missing WIR numeric-for path evidence at point %d without semantic sidecars", point)
		}
	}
	if checked == 0 {
		t.Fatal("fixture did not produce a WIR numeric-for check point")
	}
}

func TestLowerWithWIRNumericForPublishesLoopVariableRootAssignmentWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function scan()
	for i = 1, 10 do
		local index = i
	end
end
`)
	body := wirlower.LowerFunction("numeric-for-variable", fn, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
	loop := requireNumericForStmt(t, fn, 0)
	wantSymbol, ok := bindings.NumForSymbol(loop)
	if !ok || wantSymbol == 0 {
		t.Fatalf("missing numeric-for symbol")
	}

	for _, point := range built.Graph.RPO() {
		if !body.HasInstruction(point, wir.OpIterate) {
			continue
		}
		root, rootOK := facts.RootAssignment(point)
		if !rootOK {
			t.Fatalf("numeric-for point %d has no root assignment", point)
		}
		if root.TargetSymbol() != wantSymbol {
			t.Fatalf("numeric-for root target = %v, want loop symbol %v", root.TargetSymbol(), wantSymbol)
		}
		declared, declaredOK := root.DeclaredValue()
		if !declaredOK || !root.DeclaredValueContracts() {
			t.Fatalf("numeric-for root assignment has declared=%v contract=%v", declaredOK, root.DeclaredValueContracts())
		}
		got, ok := typevalue.TypeOf(standard.Registry(), declared)
		if !ok || !typ.TypeEquals(got, typ.Integer) {
			t.Fatalf("numeric-for declared value type = %v/%v, want integer", got, ok)
		}
		return
	}
	t.Fatal("fixture did not produce a WIR numeric-for point")
}

func TestLowerWithWIRNumericForUsesTypedBoundPathsForLoopVariableType(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function scan()
	local first: integer = 1
	local last: integer = 10
	local step: integer = 1
	for i = first, last, step do
		local index = i
	end
end
`)
	body := wirlower.LowerFunction("numeric-for-typed-bound-paths", fn, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})

	for _, point := range built.Graph.RPO() {
		if !body.HasInstruction(point, wir.OpIterate) {
			continue
		}
		root, rootOK := facts.RootAssignment(point)
		if !rootOK {
			t.Fatalf("numeric-for point %d has no root assignment", point)
		}
		declared, declaredOK := root.DeclaredValue()
		if !declaredOK || !root.DeclaredValueContracts() {
			t.Fatalf("numeric-for typed-bound root assignment has declared=%v contract=%v", declaredOK, root.DeclaredValueContracts())
		}
		got, ok := typevalue.TypeOf(standard.Registry(), declared)
		if !ok || !typ.TypeEquals(got, typ.Integer) {
			t.Fatalf("numeric-for typed-bound declared value type = %v/%v, want integer", got, ok)
		}
		return
	}
	t.Fatal("fixture did not produce a WIR numeric-for point")
}

func requireNumericForStmt(t *testing.T, fn *ast.FunctionExpr, index int) *ast.NumberForStmt {
	t.Helper()
	if fn == nil || index < 0 || index >= len(fn.Stmts) {
		t.Fatalf("numeric-for stmt index %d out of range", index)
	}
	loop, ok := fn.Stmts[index].(*ast.NumberForStmt)
	if !ok {
		t.Fatalf("stmt %d = %T, want numeric for", index, fn.Stmts[index])
	}
	return loop
}
