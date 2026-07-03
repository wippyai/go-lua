package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (p discriminatedUnionExhaustiveness) dispatchTableKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	if table.Symbol == 0 {
		return nil, diagnostic.Span{}, false
	}
	if keys, tableSpan, basePoint, ok := p.dispatchTableBaseKeysAt(result, point, table); ok {
		var ok bool
		tableSpan, ok = p.applyReachableDispatchTableAssignments(result, basePoint, point, table, keys, tableSpan)
		if !ok {
			return nil, diagnostic.Span{}, false
		}
		if result.CallMayInvalidateTrackedPathBetween(basePoint, point, table) {
			return nil, diagnostic.Span{}, false
		}
		return keys, tableSpan, true
	}
	return p.inheritedDispatchTableKeysAt(result, point, table)
}

func (p discriminatedUnionExhaustiveness) dispatchTableBaseKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, cfg.Point, bool) {
	graph := result.Graph()
	if graph == nil || table.Symbol == 0 {
		return nil, diagnostic.Span{}, 0, false
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return nil, diagnostic.Span{}, 0, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok {
			if keys, span, ok := dispatchTableReplacementKeys(result, fact, table); ok {
				return keys, span, cursor, true
			}
			_, staticKey, touches := dispatchTableAssignmentKeyForPath(result, cursor, fact, table)
			if touches && !staticKey {
				return nil, diagnostic.Span{}, 0, false
			}
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == table.Symbol && fact.Expr != nil {
			literal, ok := result.ObjectLiteral(fact.Expr)
			if !ok {
				return nil, diagnostic.Span{}, 0, false
			}
			keys, span, ok := result.ObjectLiteralStaticStringKeysAtPath(literal, table.Segments)
			if !ok {
				return nil, diagnostic.Span{}, 0, false
			}
			return keys, span, cursor, true
		}
		parent, ok := result.ImmediateDominator(cursor)
		if !ok || parent == cursor {
			return nil, diagnostic.Span{}, 0, false
		}
		cursor = parent
	}
}

func (p discriminatedUnionExhaustiveness) inheritedDispatchTableKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	summary, ok := p.dispatchTables[table.Key()]
	if !ok || len(summary.keys) == 0 {
		return nil, diagnostic.Span{}, false
	}
	keys := cloneDispatchKeySet(summary.keys)
	tableSpan, ok := p.applyReachableDispatchTableAssignments(result, result.Graph().Entry(), point, table, keys, summary.span)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	if result.CallMayInvalidateTrackedPathBetween(result.Graph().Entry(), point, table) {
		return nil, diagnostic.Span{}, false
	}
	return keys, tableSpan, true
}

func (p discriminatedUnionExhaustiveness) applyReachableDispatchTableAssignments(
	result *body.Result,
	from, point cfg.Point,
	table pathdom.Path,
	keys map[string]bool,
	tableSpan diagnostic.Span,
) (diagnostic.Span, bool) {
	graph := result.Graph()
	if graph == nil {
		return diagnostic.Span{}, false
	}
	for _, candidate := range graph.RPO() {
		if candidate == from || candidate == point {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok {
			continue
		}
		key, staticKey, touches := dispatchTableAssignmentKeyForPath(result, candidate, fact, table)
		if !touches ||
			!result.PointCanReach(from, candidate) ||
			!result.PointCanReach(candidate, point) {
			continue
		}
		if result.PointDominates(candidate, point) {
			if replacement, replacementSpan, ok := dispatchTableReplacementKeys(result, fact, table); ok {
				replaceDispatchKeySet(keys, replacement)
				tableSpan = replacementSpan
				continue
			}
			if !staticKey {
				return diagnostic.Span{}, false
			}
			keys[key] = true
			continue
		}
		if replacement, replacementSpan, ok := dispatchTableReplacementKeys(result, fact, table); ok {
			if intersectDispatchKeySet(keys, replacement) {
				tableSpan = replacementSpan
			}
			continue
		}
		if !staticKey {
			return diagnostic.Span{}, false
		}
	}
	return tableSpan, true
}

func dispatchTableReplacementKeys(result *body.Result, fact semantics.OrdinaryAssignmentFact, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	suffix, ok := dispatchTableReplacementSuffix(fact, table)
	if !ok || fact.Value == nil {
		return nil, diagnostic.Span{}, false
	}
	literal, ok := result.ObjectLiteral(fact.Value)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	return result.ObjectLiteralStaticStringKeysAtPath(literal, suffix)
}

func dispatchTableReplacementSuffix(fact semantics.OrdinaryAssignmentFact, table pathdom.Path) ([]segment.Segment, bool) {
	if table.Symbol == 0 {
		return nil, false
	}
	if fact.HasSymbol && fact.Symbol == table.Symbol {
		return append([]segment.Segment(nil), table.Segments...), true
	}
	if !fact.HasPath {
		return nil, false
	}
	if fact.Path.Equal(table) {
		return nil, true
	}
	if table.HasPrefix(fact.Path) {
		suffix := table.Segments[len(fact.Path.Segments):]
		return append([]segment.Segment(nil), suffix...), true
	}
	return nil, false
}

func replaceDispatchKeySet(target map[string]bool, replacement map[string]bool) {
	for key := range target {
		delete(target, key)
	}
	for key, present := range replacement {
		target[key] = present
	}
}

func intersectDispatchKeySet(target map[string]bool, other map[string]bool) bool {
	changed := false
	for key := range target {
		if !other[key] {
			delete(target, key)
			changed = true
		}
	}
	return changed
}

func dispatchTableAssignmentKeyForPath(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact, table pathdom.Path) (key string, staticKey bool, touches bool) {
	if table.Symbol == 0 {
		return "", false, false
	}
	if fact.HasPath {
		if fact.Path.HasPrefix(table) {
			suffix := fact.Path.Segments[len(table.Segments):]
			if len(suffix) != 1 {
				return "", false, true
			}
			if key, ok := segmentStringKey(suffix[0]); ok {
				return key, true, true
			}
			if key, ok := dispatchTableDynamicAssignmentKey(result, point, fact, table); ok {
				return key, true, true
			}
			return "", false, true
		}
		if table.HasPrefix(fact.Path) {
			return "", false, true
		}
	}
	if fact.HasSymbol && fact.Symbol == table.Symbol {
		return "", false, true
	}
	if fact.HasContainerPath && table.Overlaps(fact.ContainerPath) {
		if key, ok := dispatchTableDynamicAssignmentKey(result, point, fact, table); ok {
			return key, true, true
		}
		return "", false, true
	}
	return "", false, false
}

func dispatchTableDynamicAssignmentKey(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact, table pathdom.Path) (string, bool) {
	if result == nil || fact.Target == nil || table.IsEmpty() {
		return "", false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return "", false
	}
	container, ok := result.ExpressionPath(attr.Object)
	if !ok || container.IsEmpty() {
		return "", false
	}
	if !container.Equal(table) && !result.PathsEquivalentAtBoundary(point, container, table) {
		return "", false
	}
	return result.StaticStringExprValueAtBoundary(point, attr.Key)
}
