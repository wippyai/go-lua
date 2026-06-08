package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExplanationGraphRecordsDiagnosticProvenanceWithoutSummaryPayload(t *testing.T) {
	key := summary.NewDefaultKey(summary.FuncRef{GraphID: 7}, nil)
	value := product.FromType(typ.String)

	var graph ExplanationGraph
	summaryNode := graph.AddNode(SummaryKeyNode(key, "callee summary key used"))
	pointNode := graph.AddNode(PointNode(cfg.Point(11), "call site"))
	valueNode := graph.AddNode(ValueFactNode(value, "return slot 0"))
	graph.AddEdge(summaryNode, valueNode, EdgeProjectedFromSummary)
	graph.AddEdge(pointNode, valueNode, EdgeRebasedThroughCallBoundary)

	nodes := graph.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(nodes))
	}
	if !nodes[0].HasKey || nodes[0].Key != key {
		t.Fatalf("summary-key node = %#v, want recorded key", nodes[0])
	}
	if nodes[0].Value.IsZero() == false {
		t.Fatalf("summary-key node carried semantic value: %#v", nodes[0])
	}
	edges := graph.Edges()
	if len(edges) != 2 || edges[0].Kind != EdgeProjectedFromSummary {
		t.Fatalf("edges = %#v, want projected-from-summary edge first", edges)
	}
}

func TestExplanationGraphDefensivelyCopiesSlices(t *testing.T) {
	var graph ExplanationGraph
	id := graph.AddNode(ExplanationNode{Kind: NodeMissingProof, Label: "obligation missing"})
	graph.AddEdge(id, id, EdgeRejectedBecauseUnproved)

	nodes := graph.Nodes()
	nodes[0].Label = "mutated"
	if graph.Nodes()[0].Label != "obligation missing" {
		t.Fatal("Nodes exposed mutable backing")
	}
	edges := graph.Edges()
	edges[0].Kind = EdgeLostBecauseTop
	if graph.Edges()[0].Kind != EdgeRejectedBecauseUnproved {
		t.Fatal("Edges exposed mutable backing")
	}
}
