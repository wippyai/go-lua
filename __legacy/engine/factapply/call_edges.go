package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type callOutcomeSite struct {
	point cfg.Point
	site  factflow.CallSiteView
}

type callOutcomeTraversalStats struct {
	callSiteBuilds        int
	callSitePointProbes   int
	presenceDirectLookups int
}

type callOutcomeTraversalCache struct {
	graphID        uint64
	hasGraph       bool
	rpo            []cfg.Point
	callSitesBuilt bool
	callSites      []callOutcomeSite
	stats          *callOutcomeTraversalStats
}

func (c *callOutcomeTraversalCache) resetForGraph(graph cfg.Graph) {
	if c == nil || graph == nil {
		return
	}
	id := graph.ID()
	if c.hasGraph && c.graphID == id {
		return
	}
	stats := c.stats
	*c = callOutcomeTraversalCache{graphID: id, hasGraph: true, stats: stats}
}

func (c *callOutcomeTraversalCache) graphRPO(graph cfg.Graph) []cfg.Point {
	if graph == nil {
		return nil
	}
	c.resetForGraph(graph)
	if c.rpo == nil {
		c.rpo = graph.RPO()
	}
	return c.rpo
}

func (c *callOutcomeTraversalCache) exactCallSites(graph cfg.Graph, facts factflow.Facts) []callOutcomeSite {
	if graph == nil {
		return nil
	}
	c.resetForGraph(graph)
	if c.callSitesBuilt {
		return c.callSites
	}
	c.callSitesBuilt = true
	rpo := c.graphRPO(graph)
	if c.stats != nil {
		c.stats.callSiteBuilds++
		c.stats.callSitePointProbes += len(rpo)
	}
	if !facts.HasCallSites() {
		return nil
	}
	c.callSites = make([]callOutcomeSite, 0, facts.CallSiteCount())
	for _, point := range rpo {
		if site, ok := facts.CallSiteView(point); ok {
			c.callSites = append(c.callSites, callOutcomeSite{point: point, site: site})
		}
	}
	return c.callSites
}

func pathsMatchForBranchRelation(left, right pathdom.Path) bool {
	if left.Symbol != 0 || right.Symbol != 0 {
		if left.Symbol != right.Symbol || left.Version != 0 && right.Version != 0 && left.Version != right.Version {
			return false
		}
	} else if left.Root != right.Root {
		return false
	}
	if len(left.Segments) != len(right.Segments) {
		return false
	}
	for index := range left.Segments {
		leftSegment, rightSegment := left.Segments[index], right.Segments[index]
		if leftSegment.Kind != rightSegment.Kind || leftSegment.Name != rightSegment.Name || leftSegment.Index != rightSegment.Index {
			return false
		}
	}
	return true
}
