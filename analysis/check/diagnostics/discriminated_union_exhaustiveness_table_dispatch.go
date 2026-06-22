package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type dispatchTableEvidence struct {
	table      string
	target     string
	possible   []string
	keys       []string
	missing    []string
	missingFor []string
	tableSpan  diagnostic.Span
	lookupSpan diagnostic.Span
}

type dispatchTableSummary struct {
	table string
	path  pathdom.Path
	keys  map[string]bool
	span  diagnostic.Span
}

func collectDispatchTableSummaries(result *body.Result, flow *diagnosticFlowCache, inherited map[pathdom.PathKey]dispatchTableSummary) map[pathdom.PathKey]dispatchTableSummary {
	out := cloneDispatchTableSummaries(inherited)
	graph := result.Graph()
	if graph == nil {
		return out
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Expr != nil {
			literal, literalOK := result.ObjectLiteral(fact.Expr)
			if literalOK {
				base := pathdom.NewPath(fact.Symbol, result.SymbolName(fact.Symbol))
				collectObjectLiteralDispatchTableSummaries(result, &out, base, literal)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			updateDispatchTableSummariesForAssignment(result, out, point, fact)
		}
		if _, ok := result.Call(point); ok {
			updateDispatchTableSummariesForCall(result, out, point)
		}
	}
	return out
}

func collectObjectLiteralDispatchTableSummaries(result *body.Result, out *map[pathdom.PathKey]dispatchTableSummary, table pathdom.Path, fact semantics.ObjectLiteralFact) {
	if result == nil || out == nil || table.IsEmpty() {
		return
	}
	if keys, ok := objectLiteralDispatchKeys(fact); ok {
		if *out == nil {
			*out = make(map[pathdom.PathKey]dispatchTableSummary, 1)
		}
		(*out)[table.Key()] = dispatchTableSummary{
			table: table.String(),
			path:  table.Clone(),
			keys:  keys,
			span:  ast.SpanOf(fact.Table),
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
		collectObjectLiteralDispatchTableSummaries(result, out, table.AppendSegments(entry.Suffix.Segments), nested)
	}
}

func cloneDispatchTableSummaries(in map[pathdom.PathKey]dispatchTableSummary) map[pathdom.PathKey]dispatchTableSummary {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]dispatchTableSummary, len(in))
	for key, summary := range in {
		summary.path = summary.path.Clone()
		summary.keys = cloneDispatchKeySet(summary.keys)
		out[key] = summary
	}
	return out
}

func cloneDispatchKeySet(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, present := range in {
		out[key] = present
	}
	return out
}

func updateDispatchTableSummariesForAssignment(result *body.Result, summaries map[pathdom.PathKey]dispatchTableSummary, point cfg.Point, fact semantics.OrdinaryAssignmentFact) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		key, staticKey, touches := dispatchTableAssignmentKeyForPath(result, point, fact, summary.path)
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

func updateDispatchTableSummariesForCall(result *body.Result, summaries map[pathdom.PathKey]dispatchTableSummary, point cfg.Point) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if callMayInvalidateTrackedPath(result, point, summary.path) {
			delete(summaries, summaryKey)
		}
	}
}

type dispatchLookup struct {
	point        cfg.Point
	expr         *ast.AttrGetExpr
	table        pathdom.Path
	discriminant pathdom.Path
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		for _, lookup := range p.dispatchLookupsAt(result, point) {
			if diag, ok := p.tableDispatchDiagnostic(result, lookup); ok {
				out = append(out, diag)
			}
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsAt(result *body.Result, point cfg.Point) []dispatchLookup {
	var out []dispatchLookup
	if fact, ok := result.LocalAssignment(point); ok && fact.Expr != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Expr, false)...)
	}
	if fact, ok := result.OrdinaryAssignment(point); ok && fact.Value != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Value, false)...)
	}
	if call, ok := result.Call(point); ok && call.Call != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, call.Func, true)...)
		if call.Receiver != nil {
			out = append(out, p.dispatchLookupsInExpr(result, point, call.Receiver, true)...)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsInExpr(result *body.Result, point cfg.Point, expr ast.Expr, scanCallFunc bool) []dispatchLookup {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		var out []dispatchLookup
		if lookup, ok := p.dispatchLookupFromAttr(result, point, e); ok {
			out = append(out, lookup)
		}
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Object, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Key, scanCallFunc)...)
		return out
	case *ast.FuncCallExpr:
		if !scanCallFunc {
			return nil
		}
		var out []dispatchLookup
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Func, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Receiver, scanCallFunc)...)
		return out
	case *ast.CastExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.NonNilAssertExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.LogicalOpExpr:
		return nil
	default:
		return nil
	}
}

func (p discriminatedUnionExhaustiveness) dispatchLookupFromAttr(result *body.Result, point cfg.Point, attr *ast.AttrGetExpr) (dispatchLookup, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return dispatchLookup{}, false
	}
	tablePath, ok := result.ExpressionPath(attr.Object)
	if !ok || tablePath.Symbol == 0 {
		return dispatchLookup{}, false
	}
	discriminantPath, ok := result.ExpressionPath(attr.Key)
	if !ok || discriminantPath.Symbol == 0 || len(discriminantPath.Segments) == 0 {
		return dispatchLookup{}, false
	}
	return dispatchLookup{
		point:        point,
		expr:         attr,
		table:        tablePath,
		discriminant: discriminantPath,
	}, true
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostic(result *body.Result, lookup dispatchLookup) (diagnostic.Diagnostic, bool) {
	cases, ok := p.stringDiscriminantCases(result, lookup.point, lookup.discriminant)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	keys, tableSpan, ok := p.dispatchTableKeysAt(result, lookup.point, lookup.table)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	var possible []string
	var presentKeys []string
	var missingCases []string
	var missingKeys []string
	for _, c := range cases {
		possible = append(possible, c.name)
		if keys[c.key] {
			presentKeys = append(presentKeys, dispatchKeyName(lookup.table.String(), c.key))
			continue
		}
		missingCases = append(missingCases, c.name)
		missingKeys = append(missingKeys, dispatchKeyName(lookup.table.String(), c.key))
	}
	if len(missingKeys) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	sort.Strings(presentKeys)
	lookupSpan := ast.SpanOf(lookup.expr)
	return newDispatchTableExhaustivenessDiagnostic(dispatchTableEvidence{
		table:      lookup.table.String(),
		target:     lookup.discriminant.String(),
		possible:   possible,
		keys:       presentKeys,
		missing:    missingKeys,
		missingFor: missingCases,
		tableSpan:  tableSpan,
		lookupSpan: lookupSpan,
	}), true
}

func (p discriminatedUnionExhaustiveness) dispatchTableKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	if table.Symbol == 0 {
		return nil, diagnostic.Span{}, false
	}
	if keys, tableSpan, basePoint, ok := p.dispatchTableBaseKeysAt(result, point, table); ok {
		if !p.applyReachableDispatchTableAssignments(result, basePoint, point, table, keys) {
			return nil, diagnostic.Span{}, false
		}
		if p.trackedPathMayBeInvalidatedBetween(result, result.Graph(), basePoint, point, table) {
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
	var idom map[cfg.Point]cfg.Point
	if p.flow != nil && p.flow.graph == graph {
		idom = p.flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
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
			keys, span, ok := objectLiteralDispatchKeysAtPath(result, literal, table.Segments)
			if !ok {
				return nil, diagnostic.Span{}, 0, false
			}
			return keys, span, cursor, true
		}
		parent, ok := idom[cursor]
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
	if !p.applyReachableDispatchTableAssignments(result, result.Graph().Entry(), point, table, keys) {
		return nil, diagnostic.Span{}, false
	}
	if p.trackedPathMayBeInvalidatedBetween(result, result.Graph(), result.Graph().Entry(), point, table) {
		return nil, diagnostic.Span{}, false
	}
	return keys, summary.span, true
}

func (p discriminatedUnionExhaustiveness) trackedPathMayBeInvalidatedBetween(result *body.Result, graph cfg.Graph, from, to cfg.Point, target pathdom.Path) bool {
	if result == nil || graph == nil || target.IsEmpty() {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !diagnosticCanReach(p.flow, graph, from, candidate) || !diagnosticCanReach(p.flow, graph, candidate, to) {
			continue
		}
		if callMayInvalidateTrackedPath(result, candidate, target) {
			return true
		}
	}
	return false
}

func objectLiteralDispatchKeys(fact semantics.ObjectLiteralFact) (map[string]bool, bool) {
	if fact.Table == nil {
		return nil, false
	}
	keys := make(map[string]bool, len(fact.Table.Fields))
	arrayIndex := 0
	for _, field := range fact.Table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			return nil, false
		}
		switch suffix.Kind {
		case pathexpr.TableFieldSuffixField, pathexpr.TableFieldSuffixStringIndex:
			if suffix.Segment.Name == "" {
				return nil, false
			}
			keys[suffix.Segment.Name] = true
		case pathexpr.TableFieldSuffixImplicitIndex, pathexpr.TableFieldSuffixIntIndex:
			continue
		default:
			return nil, false
		}
	}
	return keys, true
}

func objectLiteralDispatchKeysAtPath(result *body.Result, fact semantics.ObjectLiteralFact, suffix []segment.Segment) (map[string]bool, diagnostic.Span, bool) {
	if len(suffix) == 0 {
		keys, ok := objectLiteralDispatchKeys(fact)
		return keys, ast.SpanOf(fact.Table), ok
	}
	nested, ok := nestedObjectLiteralFact(result, fact, suffix)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	keys, ok := objectLiteralDispatchKeys(nested)
	return keys, ast.SpanOf(nested.Table), ok
}

func nestedObjectLiteralFact(result *body.Result, fact semantics.ObjectLiteralFact, suffix []segment.Segment) (semantics.ObjectLiteralFact, bool) {
	if result == nil || len(suffix) == 0 {
		return semantics.ObjectLiteralFact{}, false
	}
	for _, entry := range fact.Entries {
		if !sameSegments(entry.Suffix.Segments, suffix) {
			continue
		}
		nested, ok := result.ObjectLiteral(entry.Value)
		return nested, ok
	}
	return semantics.ObjectLiteralFact{}, false
}

func (p discriminatedUnionExhaustiveness) applyReachableDispatchTableAssignments(result *body.Result, from, point cfg.Point, table pathdom.Path, keys map[string]bool) bool {
	graph := result.Graph()
	if graph == nil {
		return false
	}
	var idom map[cfg.Point]cfg.Point
	if p.flow != nil && p.flow.graph == graph {
		idom = p.flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	if len(idom) == 0 {
		return false
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
			!diagnosticCanReach(p.flow, graph, from, candidate) ||
			!diagnosticCanReach(p.flow, graph, candidate, point) {
			continue
		}
		if dominance.Dominates(idom, candidate, point) {
			if replacement, _, ok := dispatchTableReplacementKeys(result, fact, table); ok {
				replaceDispatchKeySet(keys, replacement)
				continue
			}
			if !staticKey {
				return false
			}
			keys[key] = true
			continue
		}
		if replacement, _, ok := dispatchTableReplacementKeys(result, fact, table); ok {
			intersectDispatchKeySet(keys, replacement)
			continue
		}
		if !staticKey {
			return false
		}
	}
	return true
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
	return objectLiteralDispatchKeysAtPath(result, literal, suffix)
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

func intersectDispatchKeySet(target map[string]bool, other map[string]bool) {
	for key := range target {
		if !other[key] {
			delete(target, key)
		}
	}
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
	return staticStringExprValueAt(result, point, attr.Key)
}
