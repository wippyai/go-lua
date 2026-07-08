package factquery

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// OrdinaryRootWriteLookup reports whether point writes target's root value.
type OrdinaryRootWriteLookup func(point cfg.Point, target symbol.ID) bool

// DominatingOrdinaryRootWriteQuery answers root-replacement dominance questions
// without coupling the query algorithm to one fact carrier.
type DominatingOrdinaryRootWriteQuery struct {
	hasWrite  OrdinaryRootWriteLookup
	idom      map[cfg.Point]cfg.Point
	graphSize int
}

// NewDominatingOrdinaryRootWriteQuery builds a reusable root-write query for graph.
func NewDominatingOrdinaryRootWriteQuery(graph cfg.Graph, hasWrite OrdinaryRootWriteLookup) DominatingOrdinaryRootWriteQuery {
	if graph == nil {
		return DominatingOrdinaryRootWriteQuery{hasWrite: hasWrite}
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	if dominators == nil {
		return DominatingOrdinaryRootWriteQuery{hasWrite: hasWrite}
	}
	return DominatingOrdinaryRootWriteQuery{
		hasWrite:  hasWrite,
		idom:      dominators.Map(),
		graphSize: graph.Size(),
	}
}

func newDominatingOrdinaryRootWriteQueryWithDominators(
	idom map[cfg.Point]cfg.Point,
	graphSize int,
	hasWrite OrdinaryRootWriteLookup,
) DominatingOrdinaryRootWriteQuery {
	return DominatingOrdinaryRootWriteQuery{hasWrite: hasWrite, idom: idom, graphSize: graphSize}
}

// DominatingOrdinaryRootWrite reports the closest ordinary write to target's
// root on the immediate-dominator chain ending at point.
func (q DominatingOrdinaryRootWriteQuery) DominatingOrdinaryRootWrite(point cfg.Point, target symbol.ID) (cfg.Point, bool) {
	if point == 0 || target == 0 || q.idom == nil || q.hasWrite == nil {
		return 0, false
	}
	visited := make(map[cfg.Point]struct{}, q.graphSize)
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return 0, false
		}
		visited[cursor] = struct{}{}
		if q.hasWrite(cursor, target) {
			return cursor, true
		}
		parent, ok := q.idom[cursor]
		if !ok || parent == cursor {
			return 0, false
		}
		cursor = parent
	}
}
