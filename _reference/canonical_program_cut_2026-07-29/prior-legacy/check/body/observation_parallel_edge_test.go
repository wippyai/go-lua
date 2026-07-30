package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestObservationSealJoinsParallelOppositeEdgeOccurrences(t *testing.T) {
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, join, true)
	graph.AddEdge(branch, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	plan := compileObservationPlan(graph, factflow.NewFacts(factflow.FactsInput{}))
	count := 0
	for _, edge := range plan.edgeReachability {
		if edge.from == branch && edge.to == join {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("observation plan retained %d duplicate pair records, want one aggregate", count)
	}

}
