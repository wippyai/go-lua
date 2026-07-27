package front

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestReachabilityWithoutSharesOneWalkAcrossTargets(t *testing.T) {
	graph := cfg.NewWithCapacity(258, 257)
	points := make([]cfg.Point, 256)
	for index := range points {
		points[index] = graph.AddNode(cfg.NodeAssign)
	}
	graph.AddEdge(graph.Entry(), points[0], false)
	for index := 1; index < len(points); index++ {
		graph.AddEdge(points[index-1], points[index], false)
	}
	graph.AddEdge(points[len(points)-1], graph.Exit(), false)

	cache := newReachabilityCache(graph)
	for _, target := range points {
		if !cache.reachesWithout(graph.Entry(), target, graph.Exit()) {
			t.Fatalf("entry does not reach %d before exit", target)
		}
	}
	if got := len(cache.without); got != 1 {
		t.Fatalf("cached excluded walks = %d, want one for one (from, exclude) pair", got)
	}
}
