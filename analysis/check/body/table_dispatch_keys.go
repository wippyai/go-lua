package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type tableDispatchSummary struct {
	path path.Path
	keys map[string]bool
	span SourceSpan
}

// TableDispatchKeysAt returns the static string keys proved present for a
// dispatch table at point. The proof owns the CFG/dominance walk so readmodel
// callers only project the solved key set.
func (r *Result) TableDispatchKeysAt(point cfg.Point, table path.Path, parents ...*Result) (map[string]bool, SourceSpan, bool) {
	if r == nil || table.Symbol == 0 {
		return nil, SourceSpan{}, false
	}
	if keys, tableSpan, basePoint, ok := r.tableDispatchBaseKeysAt(point, table); ok {
		var spanOK bool
		tableSpan, spanOK = r.applyReachableTableDispatchAssignments(basePoint, point, table, keys, tableSpan)
		if !spanOK {
			return nil, SourceSpan{}, false
		}
		if r.CallMayInvalidateTrackedPathBetween(basePoint, point, table) {
			return nil, SourceSpan{}, false
		}
		return cloneTableDispatchKeySet(keys), tableSpan, true
	}
	return r.inheritedTableDispatchKeysAt(point, table, parents)
}

func (r *Result) tableDispatchBaseKeysAt(point cfg.Point, table path.Path) (map[string]bool, SourceSpan, cfg.Point, bool) {
	graph := r.Graph()
	if graph == nil || table.Symbol == 0 {
		return nil, SourceSpan{}, 0, false
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return nil, SourceSpan{}, 0, false
		}
		visited[cursor] = struct{}{}
		if write, ok := r.LoweredAssignmentWrite(cursor); ok {
			if keys, span, ok := r.tableDispatchReplacementKeys(write, table); ok {
				return keys, span, cursor, true
			}
			_, staticKey, touches := tableDispatchAssignmentKeyForWrite(write, table)
			if touches && !staticKey {
				return nil, SourceSpan{}, 0, false
			}
		}
		if fact, ok := r.LoweredLocalAssignment(cursor); ok && fact.TargetSymbol() == table.Symbol && len(fact.TargetPathRef().Segments) == 0 {
			keys, span, ok := r.ObjectLiteralStaticStringKeysAtSource(fact.Source(), table.Segments)
			if !ok {
				return nil, SourceSpan{}, 0, false
			}
			return keys, span, cursor, true
		}
		parent, ok := r.ImmediateDominator(cursor)
		if !ok || parent == cursor {
			return nil, SourceSpan{}, 0, false
		}
		cursor = parent
	}
}

func (r *Result) inheritedTableDispatchKeysAt(point cfg.Point, table path.Path, parents []*Result) (map[string]bool, SourceSpan, bool) {
	summaries := inheritedTableDispatchSummaries(parents)
	summary, ok := summaries[table.Key()]
	if !ok || len(summary.keys) == 0 || r.Graph() == nil {
		return nil, SourceSpan{}, false
	}
	keys := cloneTableDispatchKeySet(summary.keys)
	tableSpan, ok := r.applyReachableTableDispatchAssignments(r.Graph().Entry(), point, table, keys, summary.span)
	if !ok {
		return nil, SourceSpan{}, false
	}
	if r.CallMayInvalidateTrackedPathBetween(r.Graph().Entry(), point, table) {
		return nil, SourceSpan{}, false
	}
	return keys, tableSpan, true
}

func inheritedTableDispatchSummaries(parents []*Result) map[path.PathKey]tableDispatchSummary {
	var summaries map[path.PathKey]tableDispatchSummary
	for _, parent := range parents {
		summaries = tableDispatchSummaries(parent, summaries)
	}
	return summaries
}

func tableDispatchSummaries(result *Result, inherited map[path.PathKey]tableDispatchSummary) map[path.PathKey]tableDispatchSummary {
	out := cloneTableDispatchSummaries(inherited)
	if result == nil || result.Graph() == nil {
		return out
	}
	for _, point := range cfg.RPOReadOnly(result.Graph()) {
		if fact, ok := result.LoweredLocalAssignment(point); ok && fact.TargetSymbol() != 0 && len(fact.TargetPathRef().Segments) == 0 {
			base := path.NewPath(fact.TargetSymbol(), result.SymbolName(fact.TargetSymbol()))
			collectObjectLiteralTableDispatchSummaries(result, &out, base, fact.Source())
		}
		if write, ok := result.LoweredAssignmentWrite(point); ok {
			updateTableDispatchSummariesForAssignment(result, out, write)
		}
		if _, ok := result.CallSiteView(point); ok {
			updateTableDispatchSummariesForCall(result, out, point)
		}
	}
	return out
}

func collectObjectLiteralTableDispatchSummaries(result *Result, out *map[path.PathKey]tableDispatchSummary, table path.Path, source factflow.ValueSource) {
	if result == nil || out == nil || table.IsEmpty() {
		return
	}
	literal, ok := result.ObjectLiteralViewForSource(source)
	if !ok {
		return
	}
	if keys, span, ok := result.ObjectLiteralStaticStringKeysAtSource(source, nil); ok {
		if *out == nil {
			*out = make(map[path.PathKey]tableDispatchSummary, 1)
		}
		(*out)[table.Key()] = tableDispatchSummary{
			path: table.Clone(),
			keys: keys,
			span: span,
		}
	}
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() == 0 {
			return true
		}
		if _, ok := result.ObjectLiteralViewForSource(entry.Source()); !ok {
			return true
		}
		suffix := entry.SuffixSegments()
		collectObjectLiteralTableDispatchSummaries(result, out, table.AppendSegments(suffix), entry.Source())
		return true
	})
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

func updateTableDispatchSummariesForAssignment(result *Result, summaries map[path.PathKey]tableDispatchSummary, write LoweredAssignmentWrite) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if replacement, replacementSpan, ok := result.tableDispatchReplacementKeys(write, summary.path); ok {
			summary.keys = replacement
			summary.span = replacementSpan
			summaries[summaryKey] = summary
			continue
		}
		key, staticKey, touches := tableDispatchAssignmentKeyForWrite(write, summary.path)
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

func updateTableDispatchSummariesForCall(result *Result, summaries map[path.PathKey]tableDispatchSummary, point cfg.Point) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if result.CallMayInvalidateTrackedPath(point, summary.path) {
			delete(summaries, summaryKey)
		}
	}
}

func (r *Result) applyReachableTableDispatchAssignments(
	from, point cfg.Point,
	table path.Path,
	keys map[string]bool,
	tableSpan SourceSpan,
) (SourceSpan, bool) {
	graph := r.Graph()
	if graph == nil {
		return SourceSpan{}, false
	}
	for _, candidate := range cfg.RPOReadOnly(graph) {
		if candidate == from || candidate == point {
			continue
		}
		write, ok := r.LoweredAssignmentWrite(candidate)
		if !ok {
			continue
		}
		key, staticKey, touches := tableDispatchAssignmentKeyForWrite(write, table)
		if !touches ||
			!r.PointCanReach(from, candidate) ||
			!r.PointCanReach(candidate, point) {
			continue
		}
		if r.PointDominates(candidate, point) {
			if replacement, replacementSpan, ok := r.tableDispatchReplacementKeys(write, table); ok {
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
		if replacement, replacementSpan, ok := r.tableDispatchReplacementKeys(write, table); ok {
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

func (r *Result) tableDispatchReplacementKeys(write LoweredAssignmentWrite, table path.Path) (map[string]bool, SourceSpan, bool) {
	suffix, ok := tableDispatchReplacementSuffix(write.Target, table)
	if !ok {
		return nil, SourceSpan{}, false
	}
	return r.ObjectLiteralStaticStringKeysAtSource(write.Source, suffix)
}

func tableDispatchReplacementSuffix(target path.Path, table path.Path) ([]segment.Segment, bool) {
	if target.IsEmpty() || target.Symbol == 0 || table.Symbol == 0 || target.Symbol != table.Symbol {
		return nil, false
	}
	if target.Equal(table) {
		return nil, true
	}
	if table.HasPrefix(target) {
		suffix := table.Segments[len(target.Segments):]
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

func tableDispatchAssignmentKeyForWrite(write LoweredAssignmentWrite, table path.Path) (key string, staticKey bool, touches bool) {
	if table.Symbol == 0 {
		return "", false, false
	}
	if !write.Target.IsEmpty() {
		if write.Target.HasPrefix(table) {
			suffix := write.Target.Segments[len(table.Segments):]
			if len(suffix) == 1 {
				if key, ok := tableDispatchSegmentStringKey(suffix[0]); ok {
					return key, true, true
				}
			}
			return "", false, true
		}
		if table.HasPrefix(write.Target) {
			return "", false, true
		}
	}
	if write.HasContainer && table.Overlaps(write.Container) {
		return "", false, true
	}
	return "", false, false
}

func tableDispatchSegmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
