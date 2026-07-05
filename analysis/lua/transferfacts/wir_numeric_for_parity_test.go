package transferfacts

import (
	"reflect"
	"testing"

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
