package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanInternsEqualSignatureCallDescriptors(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	graph.AddNode(cfg.NodeCall)
	last := graph.AddNode(cfg.NodeCall)
	plan := New(graph, factflow.FactsInput{})
	sig := signature.Function{Type: typ.Func().Returns(typ.String).Build(), Effect: effect.Row{}}
	op, ok := NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("signature operation rejected")
	}
	plan = plan.WithSignatureCalls(map[cfg.Point]SignatureCallOperation{first: op, last: op})
	if len(plan.signatures) != 1 {
		t.Fatalf("interned signatures = %d, want 1", len(plan.signatures))
	}
	for _, point := range []cfg.Point{first, last} {
		got, exists := plan.SignatureCallOperation(point)
		if !exists || !got.Signature().Equals(sig) {
			t.Fatalf("point %d lost signature descriptor", point)
		}
	}
	if _, exists := plan.SignatureCallOperation(first + 1); exists {
		t.Fatal("point without signature unexpectedly resolved")
	}
}
