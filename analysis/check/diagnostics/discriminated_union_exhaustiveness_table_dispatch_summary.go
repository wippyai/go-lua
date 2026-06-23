package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type dispatchTableSummary struct {
	table string
	path  pathdom.Path
	keys  map[string]bool
	span  diagnostic.Span
}

func collectDispatchTableSummaries(result *body.Result, inherited map[pathdom.PathKey]dispatchTableSummary) map[pathdom.PathKey]dispatchTableSummary {
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
