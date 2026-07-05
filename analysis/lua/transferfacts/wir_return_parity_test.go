package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLowerWithWIRReturnPointsMatchesSidecarReturnPresence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(ok: boolean): (string?, string?)
    if ok then
        return "value", nil
    end
    return nil, "error"
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	sidecarPoints := semanticReturnFactPoints(built.Graph, result)
	wirPoints := wirReturnFactPoints(built.Graph, body)
	if !reflect.DeepEqual(wirPoints, sidecarPoints) {
		t.Fatalf("return points mismatch\n got: %#v\nwant: %#v", wirPoints, sidecarPoints)
	}
	for _, point := range built.Graph.RPO() {
		got := wirFacts.ReturnPresenceRelations(point)
		want := sidecarFacts.ReturnPresenceRelations(point)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("return presence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
}
