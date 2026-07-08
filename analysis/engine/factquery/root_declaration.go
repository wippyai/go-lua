package factquery

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootDeclarationSource is the source that initialized a root symbol and still
// dominates the query point without an intervening ordinary root write.
type RootDeclarationSource struct {
	Point            cfg.Point
	Source           factflow.ValueSource
	Symbol           symbol.ID
	DeclaredValue    product.Value
	HasDeclaredValue bool
}

// RootDeclarationQuery answers root-declaration dominance questions for one
// CFG/fact set. It owns the immediate-dominator chain so callers that ask
// several questions for the same body do not repeatedly recompute dominance.
type RootDeclarationQuery struct {
	facts     factflow.Facts
	idom      map[cfg.Point]cfg.Point
	graphSize int
}

// NewRootDeclarationQuery computes a reusable declaration query for graph.
func NewRootDeclarationQuery(facts factflow.Facts, graph cfg.Graph) RootDeclarationQuery {
	if graph == nil {
		return RootDeclarationQuery{facts: facts}
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	if dominators == nil {
		return RootDeclarationQuery{facts: facts}
	}
	return RootDeclarationQuery{
		facts:     facts,
		idom:      dominators.Map(),
		graphSize: graph.Size(),
	}
}

// NewRootDeclarationQueryWithDominators adapts an existing immediate-dominator
// map into a declaration query. The map must describe the same immutable CFG
// represented by graphSize.
func NewRootDeclarationQueryWithDominators(
	facts factflow.Facts,
	idom map[cfg.Point]cfg.Point,
	graphSize int,
) RootDeclarationQuery {
	return RootDeclarationQuery{facts: facts, idom: idom, graphSize: graphSize}
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
	return NewRootDeclarationQuery(facts, graph).
		DominatingRootDeclarationSource(point, target)
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
	return NewRootDeclarationQuery(facts, graph).
		DominatingPathRootDeclarationSource(point, target)
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
	return NewRootDeclarationQuery(facts, graph).
		DominatingOrdinaryRootWrite(point, target)
}

// DominatingRootDeclarationSource finds the local declaration source for target
// on this query's immediate-dominator chain.
func (q RootDeclarationQuery) DominatingRootDeclarationSource(
	point cfg.Point,
	target symbol.ID,
) (RootDeclarationSource, bool) {
	return q.dominatingDeclarationSource(point, pathdom.Path{Symbol: target}, false)
}

// DominatingPathRootDeclarationSource finds the root declaration source for
// target on this query's immediate-dominator chain.
func (q RootDeclarationQuery) DominatingPathRootDeclarationSource(
	point cfg.Point,
	target pathdom.Path,
) (RootDeclarationSource, bool) {
	return q.dominatingDeclarationSource(point, target, true)
}

// DominatingOrdinaryRootWrite reports whether an ordinary write to target's root
// dominates point on this query's immediate-dominator chain.
func (q RootDeclarationQuery) DominatingOrdinaryRootWrite(
	point cfg.Point,
	target symbol.ID,
) (cfg.Point, bool) {
	if point == 0 || target == 0 || q.idom == nil {
		return 0, false
	}
	visited := make(map[cfg.Point]struct{}, q.graphSize)
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return 0, false
		}
		visited[cursor] = struct{}{}
		assignment, ok := q.facts.RootAssignment(cursor)
		if ok && assignment.TargetSymbol() == target {
			targetPath := assignment.TargetPath()
			if assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite && len(targetPath.Segments) == 0 {
				return cursor, true
			}
		}
		parent, ok := q.idom[cursor]
		if !ok || parent == cursor {
			return 0, false
		}
		cursor = parent
	}
}

func (q RootDeclarationQuery) dominatingDeclarationSource(
	point cfg.Point,
	target pathdom.Path,
	pathAware bool,
) (RootDeclarationSource, bool) {
	if point == 0 || q.idom == nil {
		return RootDeclarationSource{}, false
	}
	if target.Symbol == 0 {
		return RootDeclarationSource{}, false
	}
	visited := make(map[cfg.Point]struct{}, q.graphSize)
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return RootDeclarationSource{}, false
		}
		visited[cursor] = struct{}{}
		assignment, ok := q.facts.RootAssignment(cursor)
		if ok && assignment.TargetSymbol() == target.Symbol {
			targetPath := assignment.TargetPath()
			if !pathAware && len(targetPath.Segments) != 0 {
				parent, ok := q.idom[cursor]
				if !ok || parent == cursor {
					return RootDeclarationSource{}, false
				}
				cursor = parent
				continue
			}
			switch {
			case assignment.Kind() == factflow.RootAssignmentLocalDeclaration && len(targetPath.Segments) == 0:
				declared, hasDeclared := assignment.DeclaredAnnotationValue()
				return RootDeclarationSource{
					Point:            cursor,
					Source:           assignment.Source(),
					Symbol:           target.Symbol,
					DeclaredValue:    declared,
					HasDeclaredValue: hasDeclared,
				}, true
			case assignment.Kind() == factflow.RootAssignmentOrdinaryRootWrite:
				if !pathAware || target.HasPrefix(targetPath) {
					return RootDeclarationSource{}, false
				}
			case !pathAware || target.HasPrefix(targetPath):
				return RootDeclarationSource{}, false
			}
		}
		parent, ok := q.idom[cursor]
		if !ok || parent == cursor {
			return RootDeclarationSource{}, false
		}
		cursor = parent
	}
}
