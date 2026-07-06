package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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
	wirFacts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

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
	_, bindings, built, result := parseSemanticFunction(t, `
function scan(xs: {string})
	for i = 1, #xs do
		local current = xs[i]
	end
end
`)
	facts := Lower(result, built.Graph, Config{
		Registry: standard.Registry(),
		Bindings: bindings,
		WIR:      wir.NewBody("scan"),
	})

	var checked int
	for _, point := range built.Graph.RPO() {
		if _, ok := built.Meta.NumericFor(point); !ok {
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
	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked int
	for _, point := range built.Graph.RPO() {
		if _, ok := built.Meta.NumericFor(point); !ok {
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
