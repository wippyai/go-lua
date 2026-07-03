package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type tableDispatchSummary struct {
	path path.Path
	keys map[string]bool
	span SourceSpan
}

func (r Reader) tableDispatchKeysAt(point cfg.Point, table path.Path) (map[string]bool, SourceSpan, bool) {
	if table.Symbol == 0 {
		return nil, SourceSpan{}, false
	}
	if keys, tableSpan, basePoint, ok := r.tableDispatchBaseKeysAt(point, table); ok {
		var spanOK bool
		tableSpan, spanOK = r.applyReachableTableDispatchAssignments(basePoint, point, table, keys, tableSpan)
		if !spanOK {
			return nil, SourceSpan{}, false
		}
		if r.result.CallMayInvalidateTrackedPathBetween(basePoint, point, table) {
			return nil, SourceSpan{}, false
		}
		return keys, tableSpan, true
	}
	return r.inheritedTableDispatchKeysAt(point, table)
}

func (r Reader) tableDispatchBaseKeysAt(point cfg.Point, table path.Path) (map[string]bool, SourceSpan, cfg.Point, bool) {
	graph := r.result.Graph()
	if graph == nil || table.Symbol == 0 {
		return nil, SourceSpan{}, 0, false
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return nil, SourceSpan{}, 0, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := r.result.OrdinaryAssignment(cursor); ok {
			if keys, span, ok := r.tableDispatchReplacementKeys(fact, table); ok {
				return keys, span, cursor, true
			}
			_, staticKey, touches := r.tableDispatchAssignmentKeyForPath(cursor, fact, table)
			if touches && !staticKey {
				return nil, SourceSpan{}, 0, false
			}
		}
		if fact, ok := r.result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == table.Symbol && fact.Expr != nil {
			literal, ok := r.result.ObjectLiteral(fact.Expr)
			if !ok {
				return nil, SourceSpan{}, 0, false
			}
			keys, span, ok := r.result.ObjectLiteralStaticStringKeysAtPath(literal, table.Segments)
			if !ok {
				return nil, SourceSpan{}, 0, false
			}
			return keys, sourceSpanFromAST(span), cursor, true
		}
		parent, ok := r.result.ImmediateDominator(cursor)
		if !ok || parent == cursor {
			return nil, SourceSpan{}, 0, false
		}
		cursor = parent
	}
}

func (r Reader) inheritedTableDispatchKeysAt(point cfg.Point, table path.Path) (map[string]bool, SourceSpan, bool) {
	summaries := r.inheritedTableDispatchSummaries()
	summary, ok := summaries[table.Key()]
	if !ok || len(summary.keys) == 0 || r.result.Graph() == nil {
		return nil, SourceSpan{}, false
	}
	keys := cloneTableDispatchKeySet(summary.keys)
	tableSpan, ok := r.applyReachableTableDispatchAssignments(r.result.Graph().Entry(), point, table, keys, summary.span)
	if !ok {
		return nil, SourceSpan{}, false
	}
	if r.result.CallMayInvalidateTrackedPathBetween(r.result.Graph().Entry(), point, table) {
		return nil, SourceSpan{}, false
	}
	return keys, tableSpan, true
}

func (r Reader) inheritedTableDispatchSummaries() map[path.PathKey]tableDispatchSummary {
	var summaries map[path.PathKey]tableDispatchSummary
	for _, parent := range r.parents {
		summaries = tableDispatchSummaries(parent, summaries)
	}
	return summaries
}

func tableDispatchSummaries(result *body.Result, inherited map[path.PathKey]tableDispatchSummary) map[path.PathKey]tableDispatchSummary {
	out := cloneTableDispatchSummaries(inherited)
	if result == nil || result.Graph() == nil {
		return out
	}
	for _, point := range cfg.RPOReadOnly(result.Graph()) {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Expr != nil {
			if literal, literalOK := result.ObjectLiteral(fact.Expr); literalOK {
				base := path.NewPath(fact.Symbol, result.SymbolName(fact.Symbol))
				collectObjectLiteralTableDispatchSummaries(result, &out, base, literal)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			updateTableDispatchSummariesForAssignment(result, out, point, fact)
		}
		if _, ok := result.Call(point); ok {
			updateTableDispatchSummariesForCall(result, out, point)
		}
	}
	return out
}

func collectObjectLiteralTableDispatchSummaries(result *body.Result, out *map[path.PathKey]tableDispatchSummary, table path.Path, fact body.ObjectLiteralFact) {
	if result == nil || out == nil || table.IsEmpty() {
		return
	}
	if keys, span, ok := result.ObjectLiteralStaticStringKeysAtPath(fact, nil); ok {
		if *out == nil {
			*out = make(map[path.PathKey]tableDispatchSummary, 1)
		}
		(*out)[table.Key()] = tableDispatchSummary{
			path: table.Clone(),
			keys: keys,
			span: sourceSpanFromAST(span),
		}
	}
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) == 0 {
			continue
		}
		nested, ok := result.ObjectLiteral(entry.Value)
		if !ok {
			continue
		}
		collectObjectLiteralTableDispatchSummaries(result, out, table.AppendSegments(entry.Suffix.Segments), nested)
	}
}

func cloneTableDispatchSummaries(in map[path.PathKey]tableDispatchSummary) map[path.PathKey]tableDispatchSummary {
	if len(in) == 0 {
		return nil
	}
	out := make(map[path.PathKey]tableDispatchSummary, len(in))
	for key, summary := range in {
		summary.path = summary.path.Clone()
		summary.keys = cloneTableDispatchKeySet(summary.keys)
		out[key] = summary
	}
	return out
}

func cloneTableDispatchKeySet(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, present := range in {
		out[key] = present
	}
	return out
}

func updateTableDispatchSummariesForAssignment(result *body.Result, summaries map[path.PathKey]tableDispatchSummary, point cfg.Point, fact body.OrdinaryAssignmentFact) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		key, staticKey, touches := tableDispatchAssignmentKeyForPath(result, point, fact, summary.path)
		if !touches {
			continue
		}
		if !staticKey {
			delete(summaries, summaryKey)
			continue
		}
		if summary.keys == nil {
			summary.keys = make(map[string]bool, 1)
		}
		summary.keys[key] = true
		summaries[summaryKey] = summary
	}
}

func updateTableDispatchSummariesForCall(result *body.Result, summaries map[path.PathKey]tableDispatchSummary, point cfg.Point) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if result.CallMayInvalidateTrackedPath(point, summary.path) {
			delete(summaries, summaryKey)
		}
	}
}

func (r Reader) applyReachableTableDispatchAssignments(
	from, point cfg.Point,
	table path.Path,
	keys map[string]bool,
	tableSpan SourceSpan,
) (SourceSpan, bool) {
	graph := r.result.Graph()
	if graph == nil {
		return SourceSpan{}, false
	}
	for _, candidate := range cfg.RPOReadOnly(graph) {
		if candidate == from || candidate == point {
			continue
		}
		fact, ok := r.result.OrdinaryAssignment(candidate)
		if !ok {
			continue
		}
		key, staticKey, touches := r.tableDispatchAssignmentKeyForPath(candidate, fact, table)
		if !touches ||
			!r.result.PointCanReach(from, candidate) ||
			!r.result.PointCanReach(candidate, point) {
			continue
		}
		if r.result.PointDominates(candidate, point) {
			if replacement, replacementSpan, ok := r.tableDispatchReplacementKeys(fact, table); ok {
				replaceTableDispatchKeySet(keys, replacement)
				tableSpan = replacementSpan
				continue
			}
			if !staticKey {
				return SourceSpan{}, false
			}
			keys[key] = true
			continue
		}
		if replacement, replacementSpan, ok := r.tableDispatchReplacementKeys(fact, table); ok {
			if intersectTableDispatchKeySet(keys, replacement) {
				tableSpan = replacementSpan
			}
			continue
		}
		if !staticKey {
			return SourceSpan{}, false
		}
	}
	return tableSpan, true
}

func (r Reader) tableDispatchReplacementKeys(fact body.OrdinaryAssignmentFact, table path.Path) (map[string]bool, SourceSpan, bool) {
	suffix, ok := tableDispatchReplacementSuffix(fact, table)
	if !ok || fact.Value == nil {
		return nil, SourceSpan{}, false
	}
	literal, ok := r.result.ObjectLiteral(fact.Value)
	if !ok {
		return nil, SourceSpan{}, false
	}
	keys, span, ok := r.result.ObjectLiteralStaticStringKeysAtPath(literal, suffix)
	return keys, sourceSpanFromAST(span), ok
}

func tableDispatchReplacementSuffix(fact body.OrdinaryAssignmentFact, table path.Path) ([]segment.Segment, bool) {
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

func replaceTableDispatchKeySet(target map[string]bool, replacement map[string]bool) {
	for key := range target {
		delete(target, key)
	}
	for key, present := range replacement {
		target[key] = present
	}
}

func intersectTableDispatchKeySet(target map[string]bool, other map[string]bool) bool {
	changed := false
	for key := range target {
		if !other[key] {
			delete(target, key)
			changed = true
		}
	}
	return changed
}

func (r Reader) tableDispatchAssignmentKeyForPath(point cfg.Point, fact body.OrdinaryAssignmentFact, table path.Path) (key string, staticKey bool, touches bool) {
	return tableDispatchAssignmentKeyForPath(r.result, point, fact, table)
}

func tableDispatchAssignmentKeyForPath(result *body.Result, point cfg.Point, fact body.OrdinaryAssignmentFact, table path.Path) (key string, staticKey bool, touches bool) {
	if table.Symbol == 0 {
		return "", false, false
	}
	if fact.HasPath {
		if fact.Path.HasPrefix(table) {
			suffix := fact.Path.Segments[len(table.Segments):]
			if len(suffix) != 1 {
				return "", false, true
			}
			if key, ok := registrationSegmentStringKey(suffix[0]); ok {
				return key, true, true
			}
			if key, ok := result.StaticStringAssignmentKeyForContainer(point, fact, table); ok {
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
		if key, ok := result.StaticStringAssignmentKeyForContainer(point, fact, table); ok {
			return key, true, true
		}
		return "", false, true
	}
	return "", false, false
}
