package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// callOutcomeAssignmentKey is the immutable identity of one call-result
// assignment query retained by path-store preparation. Call correlations
// themselves are published by CallOutcomeCorrelationFactorProgram; this cache
// owns only CFG attribution of later stores.
type callOutcomeAssignmentKey struct {
	callPoint   cfg.Point
	resultIndex int
	targetIndex int
	targetPath  pathdom.PathKey
}

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
	graphID          uint64
	hasGraph         bool
	rpo              []cfg.Point
	callSitesBuilt   bool
	callSites        []callOutcomeSite
	pointOrder       map[cfg.Point]int
	assignmentPoints map[callOutcomeAssignmentKey]cfg.Point
	stats            *callOutcomeTraversalStats
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

func (c *callOutcomeTraversalCache) graphPointOrder(graph cfg.Graph) map[cfg.Point]int {
	if graph == nil {
		return nil
	}
	c.resetForGraph(graph)
	if c.pointOrder == nil {
		rpo := c.graphRPO(graph)
		c.pointOrder = make(map[cfg.Point]int, len(rpo))
		for index, point := range rpo {
			c.pointOrder[point] = index
		}
	}
	return c.pointOrder
}

func callOutcomeRelatableTarget(target factflow.CallResultTargetView) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment, factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && !target.TargetPathEmpty()
	default:
		return false
	}
}

func callOutcomeResultAssignmentPoint(
	cache *callOutcomeTraversalCache,
	graph cfg.Graph,
	facts factflow.Facts,
	callPoint cfg.Point,
	target factflow.CallResultTargetView,
	resultIndex int,
) (cfg.Point, bool) {
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	rpo := cache.graphRPO(graph)
	key := callOutcomeAssignmentKey{
		callPoint: callPoint, resultIndex: resultIndex,
		targetIndex: target.Index(), targetPath: target.TargetPathKey(),
	}
	if cache.assignmentPoints != nil {
		if point, ok := cache.assignmentPoints[key]; ok {
			return point, point != 0
		}
	} else {
		cache.assignmentPoints = make(map[callOutcomeAssignmentKey]cfg.Point)
	}
	for _, point := range rpo {
		if assignment, ok := facts.RootAssignment(point); ok &&
			target.TargetPathEqual(assignment.TargetPathRef()) &&
			callOutcomeValueSourceConsumesResult(assignment.Source(), callPoint, target, resultIndex) {
			cache.assignmentPoints[key] = point
			return point, true
		}
	}
	cache.assignmentPoints[key] = 0
	return 0, false
}

func callOutcomeValueSourceConsumesResult(
	source factflow.ValueSource,
	callPoint cfg.Point,
	target factflow.CallResultTargetView,
	resultIndex int,
) bool {
	return source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.CallPoint == callPoint &&
		source.ResultIndex == resultIndex && source.TargetIndex == target.Index()
}

func callOutcomeLaterPoint(cache *callOutcomeTraversalCache, graph cfg.Graph, first, second cfg.Point) cfg.Point {
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	order := cache.graphPointOrder(graph)
	if order[second] > order[first] {
		return second
	}
	return first
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
