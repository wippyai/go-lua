package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// dominatingRootLocalAssignment returns the nearest dominating local assignment
// that declared target's root value. A later root write on the dominator chain
// blocks the declaration because the declaration no longer explains the value
// read at point.
func dominatingRootLocalAssignment(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID) (semantics.LocalAssignmentFact, cfg.Point, bool) {
	if result == nil {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	graph := result.Graph()
	if graph == nil || target == 0 {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	var idom map[cfg.Point]cfg.Point
	if flow != nil && flow.graph == graph {
		idom = flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	var best semantics.LocalAssignmentFact
	var bestPoint cfg.Point
	found := false
	for _, candidate := range graph.RPO() {
		fact, ok := result.LocalAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target || !dominance.Dominates(idom, candidate, point) {
			continue
		}
		if !found || dominance.Dominates(idom, bestPoint, candidate) {
			best = fact
			bestPoint = candidate
			found = true
		}
	}
	if !found {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	for _, candidate := range graph.RPO() {
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != target || (fact.HasPath && len(fact.Path.Segments) != 0) {
			continue
		}
		if candidate == bestPoint {
			continue
		}
		if diagnosticCanReach(flow, graph, bestPoint, candidate) && dominance.Dominates(idom, candidate, point) {
			return semantics.LocalAssignmentFact{}, 0, false
		}
	}
	return best, bestPoint, true
}
