package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLowerWithWIRNumericForProofsPublishForBothLoopDirections(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
	for j = #xs, 1, -1 do
		local current = xs[j]
	end
end
`)
	body := wirlower.Lower("scan", fn.Stmts, bindings, built)
	wirFacts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

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
	_, bindings, built, _ := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
end
`)
	facts := Lower(built.Graph, Config{
		Registry: standard.Registry(),
		Bindings: bindings,
		WIR:      wir.NewBody("scan"),
	})

	var checked int
	for _, point := range built.Graph.RPO() {
		if _, ok := built.NumericFors.Get(point); !ok {
			continue
		}
		checked++
		if got := facts.BranchNumFloorRefinements(point); len(got) != 0 {
			t.Fatalf("WIR numeric-for num-floor proofs fell back to sidecar at point %d: %#v", point, got)
		}
		if got := facts.BranchPathEvidence(point); len(got) != 0 {
			t.Fatalf("WIR numeric-for path evidence fell back to sidecar at point %d: %#v", point, got)
		}
	}
	if checked == 0 {
		t.Fatal("fixture did not produce numeric-for points")
	}
}

func TestLowerWithWIRNumericForProofsPublishWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
end
`)
	body := wirlower.Lower("numeric-for-no-sidecars", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked int
	for _, point := range built.Graph.RPO() {
		if _, ok := built.NumericFors.Get(point); !ok {
			continue
		}
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
	fn, bindings, built, _ := parseSemanticFunction(t, `
function scan()
	for i = 1, 10 do
		local index = i
	end
end
`)
	body := wirlower.Lower("numeric-for-variable", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, point := range built.Graph.RPO() {
		fact, ok := built.NumericFors.Get(point)
		if !ok || !body.HasInstruction(point, wir.OpIterate) {
			continue
		}
		root, rootOK := facts.RootAssignment(point)
		if !rootOK {
			t.Fatalf("numeric-for point %d has no root assignment", point)
		}
		if root.TargetSymbol() != fact.Symbol {
			t.Fatalf("numeric-for root target = %v, want loop symbol %v", root.TargetSymbol(), fact.Symbol)
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
	fn, bindings, built, _ := parseSemanticFunction(t, `
function scan()
	local first: integer = 1
	local last: integer = 10
	local step: integer = 1
	for i = first, last, step do
		local index = i
	end
end
`)
	body := wirlower.Lower("numeric-for-typed-bound-paths", fn.Stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, point := range built.Graph.RPO() {
		if _, ok := built.NumericFors.Get(point); !ok || !body.HasInstruction(point, wir.OpIterate) {
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
