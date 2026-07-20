package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestCallOutcomeTraversalCacheBuildsExactCallSitesOnce(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	prev := first
	graph.AddEdge(graph.Entry(), first, false)
	for range 64 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(prev, point, false)
		prev = point
	}
	second := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(prev, second, false)
	graph.AddEdge(second, graph.Exit(), false)

	facts := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
		first:  factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		second: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
	}})
	stats := &callOutcomeTraversalStats{}
	cache := &callOutcomeTraversalCache{stats: stats}

	firstRead := cache.exactCallSites(graph, facts)
	secondRead := cache.exactCallSites(graph, facts)

	if len(firstRead) != 2 || len(secondRead) != 2 {
		t.Fatalf("exact call sites = %d, %d; want 2, 2", len(firstRead), len(secondRead))
	}
	if firstRead[0].point != first || firstRead[1].point != second {
		t.Fatalf("exact call-site order = %v, %v; want %v, %v", firstRead[0].point, firstRead[1].point, first, second)
	}
	if stats.callSiteBuilds != 1 {
		t.Fatalf("call-site cache builds = %d, want 1", stats.callSiteBuilds)
	}
	if stats.callSitePointProbes != len(graph.RPO()) {
		t.Fatalf("call-site point probes = %d, want one RPO scan of %d", stats.callSitePointProbes, len(graph.RPO()))
	}
}
