package factquery

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
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
	return dominatingDeclarationSource(point, pathdom.Path{Symbol: target}, facts, graph, false)
}

// DominatingPathRootDeclarationSource finds the root declaration source for
// target on the immediate-dominator chain. A write to the root or to an ancestor
// of target stops the search because the declaration no longer describes the
// queried path. Descendant writes below target do not stop it: mutating entries
// in a map does not replace the map container itself.
func DominatingPathRootDeclarationSource(
	point cfg.Point,
	target pathdom.Path,
	facts factflow.Facts,
	graph cfg.Graph,
) (RootDeclarationSource, bool) {
	return dominatingDeclarationSource(point, target, facts, graph, true)
}

// DominatingOrdinaryRootWrite reports whether an ordinary write to target's root
// dominates point. Descendant writes do not count: mutating target.field does not
// replace target itself or invalidate the root's declared contract.
func DominatingOrdinaryRootWrite(
	point cfg.Point,
	target symbol.ID,
	facts factflow.Facts,
	graph cfg.Graph,
) (cfg.Point, bool) {
	if point == 0 || target == 0 || graph == nil {
		return 0, false
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	if dominators == nil {
		return 0, false
	}
	idom := dominators.Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return 0, false
		}
		visited[cursor] = struct{}{}
		assignment, ok := facts.RootAssignment(cursor)
		if ok && assignment.TargetSymbol() == target {
			targetPath := assignment.TargetPath()
			if assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite && len(targetPath.Segments) == 0 {
				return cursor, true
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return 0, false
		}
		cursor = parent
	}
}

func dominatingDeclarationSource(
	point cfg.Point,
	target pathdom.Path,
	facts factflow.Facts,
	graph cfg.Graph,
	pathAware bool,
) (RootDeclarationSource, bool) {
	if point == 0 || graph == nil {
		return RootDeclarationSource{}, false
	}
	if target.Symbol == 0 {
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
		if ok && assignment.TargetSymbol() == target.Symbol {
			targetPath := assignment.TargetPath()
			if !pathAware && len(targetPath.Segments) != 0 {
				parent, ok := idom[cursor]
				if !ok || parent == cursor {
					return RootDeclarationSource{}, false
				}
				cursor = parent
				continue
			}
			switch {
			case assignment.Kind() == factflow.RootAssignmentLocalDeclaration && len(targetPath.Segments) == 0:
				return RootDeclarationSource{Point: cursor, Source: assignment.Source(), Symbol: target.Symbol}, true
			case assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite:
				if !pathAware || target.HasPrefix(targetPath) {
					return RootDeclarationSource{}, false
				}
			case !pathAware || target.HasPrefix(targetPath):
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
