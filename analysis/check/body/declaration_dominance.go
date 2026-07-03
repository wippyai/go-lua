package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DominatingRootLocalAssignment returns the nearest dominating local assignment
// that declared target's root value. A later root write on the dominator chain
// blocks the declaration because the declaration no longer explains the value
// read at point.
func (r *Result) DominatingRootLocalAssignment(point cfg.Point, target symbol.ID) (semantics.LocalAssignmentFact, cfg.Point, bool) {
	if r == nil {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	graph := r.Graph()
	if graph == nil || target == 0 {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	var best semantics.LocalAssignmentFact
	var bestPoint cfg.Point
	found := false
	for _, candidate := range graph.RPO() {
		fact, ok := r.LocalAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target || !r.PointDominates(candidate, point) {
			continue
		}
		if !found || r.PointDominates(bestPoint, candidate) {
			best = fact
			bestPoint = candidate
			found = true
		}
	}
	if !found {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	for _, candidate := range graph.RPO() {
		fact, ok := r.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target || (fact.HasPath && len(fact.Path.Segments) != 0) {
			continue
		}
		if candidate == bestPoint {
			continue
		}
		if r.PointCanReach(bestPoint, candidate) && r.PointDominates(candidate, point) {
			return semantics.LocalAssignmentFact{}, 0, false
		}
	}
	return best, bestPoint, true
}
