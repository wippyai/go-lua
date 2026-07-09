package body

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// PointCanReach reports whether the CFG has a path from from to to. This is a
// structural control-flow query, cached on Result so diagnostics and obligation
// helpers do not build parallel reachability caches.
func (r *Result) PointCanReach(from, to cfg.Point) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	reach := r.queries.controlReachability(graph)
	if reach == nil {
		return false
	}
	return reach.CanReach(from, to)
}

// ImmediateDominator returns point's immediate dominator in this result's CFG.
func (r *Result) ImmediateDominator(point cfg.Point) (cfg.Point, bool) {
	graph := r.Graph()
	if graph == nil {
		return 0, false
	}
	idom := r.queries.immediateDominatorMap(graph)
	if len(idom) == 0 {
		return 0, false
	}
	parent, ok := idom[point]
	return parent, ok
}

// PointDominates reports whether dominator dominates point in this result's CFG.
func (r *Result) PointDominates(dominator, point cfg.Point) bool {
	graph := r.Graph()
	if graph == nil {
		return false
	}
	idom := r.queries.immediateDominatorMap(graph)
	if len(idom) == 0 {
		return false
	}
	info := r.queries.immediateDominatorInfoFor(graph)
	return info != nil && info.Dominates(dominator, point)
}
