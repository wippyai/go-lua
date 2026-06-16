package factquery

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootDeclarationSource is the source that initialized a root symbol and still
// dominates the query point without an intervening ordinary root write.
type RootDeclarationSource struct {
	Point  cfg.Point
	Source factflow.ValueSource
	Symbol symbol.ID
}

// DominatingRootDeclarationSource finds the local declaration source for target
// on the immediate-dominator chain ending at point. An ordinary root write to
// the same symbol stops the search because the declaration source no longer
// describes the current root value.
func DominatingRootDeclarationSource(
	point cfg.Point,
	target symbol.ID,
	facts factflow.Facts,
	graph cfg.Graph,
) (RootDeclarationSource, bool) {
	if point == 0 || target == 0 || graph == nil {
		return RootDeclarationSource{}, false
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	if dominators == nil {
		return RootDeclarationSource{}, false
	}
	idom := dominators.Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return RootDeclarationSource{}, false
		}
		visited[cursor] = struct{}{}
		assignment, ok := facts.RootAssignment(cursor)
		if ok && assignment.TargetSymbol() == target && len(assignment.TargetPath().Segments) == 0 {
			switch assignment.Kind() {
			case factflow.RootAssignmentLocalDeclaration:
				return RootDeclarationSource{
					Point:  cursor,
					Source: assignment.Source(),
					Symbol: target,
				}, true
			case factflow.RootAssignmentOrdinaryRootWrite:
				return RootDeclarationSource{}, false
			default:
				return RootDeclarationSource{}, false
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return RootDeclarationSource{}, false
		}
		cursor = parent
	}
}
