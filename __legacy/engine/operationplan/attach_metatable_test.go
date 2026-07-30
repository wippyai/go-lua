package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestPlanInternsTypedAttachMetatableOperations(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	middle := graph.AddNode(cfg.NodeCall)
	last := graph.AddNode(cfg.NodeCall)
	table := factflow.ValueSource{Kind: factflow.ValueSourceLiteral, LiteralKind: factflow.ValueSourceLiteralString, String: "table"}
	metatable := factflow.ValueSource{Kind: factflow.ValueSourceLiteral, LiteralKind: factflow.ValueSourceLiteralString, String: "metatable"}
	op, ok := NewAttachMetatableOperation(table, metatable)
	if !ok {
		t.Fatal("typed attach-metatable operation rejected valid sources")
	}
	plan := New(graph, factflow.FactsInput{}).WithAttachMetatables(map[cfg.Point]AttachMetatableOperation{
		first: op,
		last:  op,
	})
	if len(plan.attachMetatables) != 1 {
		t.Fatalf("interned attach-metatable operations = %d, want 1", len(plan.attachMetatables))
	}
	for _, point := range []cfg.Point{first, last} {
		got, exists := plan.AttachMetatableOperation(point)
		if !exists || !factflow.ValueSourceEqual(got.Table(), table) || !factflow.ValueSourceEqual(got.Metatable(), metatable) {
			t.Fatalf("point %d lost typed attach-metatable operands", point)
		}
	}
	if _, exists := plan.AttachMetatableOperation(middle); exists {
		t.Fatal("point without attach-metatable operation unexpectedly resolved")
	}
	if invalid := New(graph, factflow.FactsInput{}).WithAttachMetatables(map[cfg.Point]AttachMetatableOperation{first: {}}); len(invalid.attachMetatables) != 0 {
		t.Fatal("invalid attach-metatable operation entered immutable plan")
	}
}
