package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLowerWithWIRNumericForProofsMatchSidecarLowering(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
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
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	direct := lowerer{bindings: bindings, wir: body}

	compared := 0
	for _, point := range built.Graph.RPO() {
		fact, ok := result.NumericFor(point)
		if !ok {
			continue
		}
		compared++
		if got, gotOK := direct.numericForBranchNumFloorRefinementFromWIR(point); gotOK {
			want, wantOK := direct.numericForBranchNumFloorRefinement(fact)
			if !wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("direct WIR numeric-for num-floor proof at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
			}
		} else if _, wantOK := direct.numericForBranchNumFloorRefinement(fact); wantOK {
			t.Fatalf("direct WIR numeric-for num-floor proof at point %d missing", point)
		}
		if got, want := direct.numericForBranchPathEvidenceFromWIR(point), direct.numericForBranchPathEvidence(fact); !reflect.DeepEqual(got, want) {
			t.Fatalf("direct WIR numeric-for path evidence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
		if got, want := wirFacts.BranchNumFloorRefinements(point), sidecarFacts.BranchNumFloorRefinements(point); !reflect.DeepEqual(got, want) {
			t.Fatalf("numeric-for num-floor proofs at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
		if got, want := wirFacts.BranchPathEvidence(point), sidecarFacts.BranchPathEvidence(point); !reflect.DeepEqual(got, want) {
			t.Fatalf("numeric-for path evidence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
	if compared != 4 {
		t.Fatalf("compared %d numeric-for points, want init/check for two loops", compared)
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
		if _, ok := result.NumericFor(point); !ok {
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
	fn, bindings, built, result := parseSemanticFunction(t, `
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
		if _, ok := result.NumericFor(point); !ok {
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
