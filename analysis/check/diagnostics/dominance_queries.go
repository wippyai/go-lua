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
func dominatingRootLocalAssignment(result *body.Result, point cfg.Point, target symbol.ID) (semantics.LocalAssignmentFact, cfg.Point, bool) {
	if result == nil {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	graph := result.Graph()
	if graph == nil || target == 0 {
		return semantics.LocalAssignmentFact{}, 0, false
	}
	idom := dominance.ComputeImmediateDominatorInfo(graph).Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return semantics.LocalAssignmentFact{}, 0, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target && (!fact.HasPath || len(fact.Path.Segments) == 0) {
			return semantics.LocalAssignmentFact{}, 0, false
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target {
			return fact, cursor, true
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return semantics.LocalAssignmentFact{}, 0, false
		}
		cursor = parent
	}
}
