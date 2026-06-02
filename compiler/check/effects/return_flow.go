package effects

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

func inferLocalReturnFlowRow(result *api.FuncResult) effect.Row {
	if result == nil || result.Graph == nil {
		return effect.Empty
	}
	params := returnFlowParamIndexes(result.Graph)
	if len(params) == 0 {
		return effect.Empty
	}
	observer := observation.FromFuncResult(result, nil)
	aliases := newReturnFlowAliasResolver(result, result.Graph, observer, params)
	var row effect.Row
	result.Graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for returnIndex, expr := range info.Exprs {
			for _, label := range returnFlowLabelsForExpr(result.Graph, observer, params, aliases, p, returnIndex, nil, expr) {
				row = effect.Union(row, effect.Row{Labels: []effect.Label{label}})
			}
		}
	})
	return row
}

func returnFlowParamIndexes(graph *cfg.Graph) map[cfg.SymbolID]int {
	slots := graph.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]int, len(slots))
	for i, slot := range slots {
		if slot.Symbol != 0 {
			out[slot.Symbol] = i
		}
	}
	return out
}

func returnFlowLabelsForExpr(
	graph *cfg.Graph,
	observer observation.Projector,
	params map[cfg.SymbolID]int,
	aliases *returnFlowAliasResolver,
	point cfg.Point,
	returnIndex int,
	target effect.PathSuffix,
	expr ast.Expr,
) []effect.FlowInto {
	if expr == nil {
		return nil
	}
	if tbl, ok := expr.(*ast.TableExpr); ok {
		var labels []effect.FlowInto
		for _, field := range tbl.Fields {
			if field == nil || field.Key == nil {
				continue
			}
			seg, ok := flowpath.StaticFieldSegment(field)
			if !ok {
				continue
			}
			nextTarget := target.Append(seg)
			labels = append(labels, returnFlowLabelsForExpr(graph, observer, params, aliases, point, returnIndex, nextTarget, field.Value)...)
		}
		return labels
	}
	candidates := returnFlowSourcesForExpr(graph, observer, params, aliases, point, expr)
	if len(candidates) == 0 {
		return nil
	}
	labels := make([]effect.FlowInto, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.ReturnIndex = returnIndex
		candidate.TargetPath = effect.PathSuffixFromSegments(target)
		labels = append(labels, candidate)
	}
	return labels
}

func returnFlowSourcesForExpr(
	graph *cfg.Graph,
	observer observation.Projector,
	params map[cfg.SymbolID]int,
	aliases *returnFlowAliasResolver,
	point cfg.Point,
	expr ast.Expr,
) []effect.FlowInto {
	if expr == nil {
		return nil
	}
	if labels := directReturnFlowSources(graph, params, aliases, point, expr); len(labels) > 0 {
		return labels
	}
	if logical, ok := expr.(*ast.LogicalOpExpr); ok && logical.Operator == "or" {
		left := returnFlowSourcesForExpr(graph, observer, params, aliases, point, logical.Lhs)
		right := returnFlowSourcesForExpr(graph, observer, params, aliases, point, logical.Rhs)
		rightType := observer.TypeOf(logical.Rhs, point)
		leftType := observer.TypeOf(logical.Lhs, point)
		for i := range left {
			left[i].Remainder = joinFlowRemainder(left[i].Remainder, rightType)
		}
		for i := range right {
			right[i].Remainder = joinFlowRemainder(right[i].Remainder, narrow.ToFalsy(leftType))
		}
		return append(left, right...)
	}
	return nil
}

func directReturnFlowSources(
	graph *cfg.Graph,
	params map[cfg.SymbolID]int,
	aliases *returnFlowAliasResolver,
	point cfg.Point,
	expr ast.Expr,
) []effect.FlowInto {
	path := flowpath.FromExprWithBindingsAt(expr, nil, graph.Bindings(), graph, point)
	if path.Symbol == 0 {
		return nil
	}
	sourcePath := effect.PathSuffixFromSegments(path.Segments)
	if paramIndex, ok := params[path.Symbol]; ok {
		return []effect.FlowInto{{ParamIndex: paramIndex, SourcePath: sourcePath}}
	}
	if alias := aliases.lookup(path.Symbol); !alias.Pure() || alias.IsOpen() {
		return appendAliasSourcePath(alias, sourcePath)
	}
	return nil
}

type returnFlowAliasResolver struct {
	graph       *cfg.Graph
	observer    observation.Projector
	params      map[cfg.SymbolID]int
	assignments map[cfg.SymbolID][]returnFlowAliasAssignment
	memo        map[cfg.SymbolID]effect.Row
	visiting    map[cfg.SymbolID]bool
}

type returnFlowAliasAssignment struct {
	point cfg.Point
	expr  ast.Expr
}

func newReturnFlowAliasResolver(
	result *api.FuncResult,
	graph *cfg.Graph,
	observer observation.Projector,
	params map[cfg.SymbolID]int,
) *returnFlowAliasResolver {
	if result == nil || len(result.Evidence.Assignments) == 0 {
		return nil
	}
	assignments := make(map[cfg.SymbolID][]returnFlowAliasAssignment)
	for _, assignment := range result.Evidence.Assignments {
		info := assignment.Info
		if info == nil {
			continue
		}
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			assignments[target.Symbol] = append(assignments[target.Symbol], returnFlowAliasAssignment{
				point: assignment.Point,
				expr:  info.SourceAt(i),
			})
		}
	}
	if len(assignments) == 0 {
		return nil
	}
	return &returnFlowAliasResolver{
		graph:       graph,
		observer:    observer,
		params:      params,
		assignments: assignments,
		memo:        make(map[cfg.SymbolID]effect.Row, len(assignments)),
		visiting:    make(map[cfg.SymbolID]bool),
	}
}

func (r *returnFlowAliasResolver) lookup(sym cfg.SymbolID) effect.Row {
	if r == nil || sym == 0 {
		return effect.Empty
	}
	if row, ok := r.memo[sym]; ok {
		return row
	}
	if r.visiting[sym] {
		return effect.Empty
	}
	r.visiting[sym] = true
	var row effect.Row
	for _, assignment := range r.assignments[sym] {
		row = effect.Union(row, rowFromReturnFlowSources(
			returnFlowSourcesForExpr(r.graph, r.observer, r.params, r, assignment.point, assignment.expr),
		))
	}
	delete(r.visiting, sym)
	r.memo[sym] = row
	return row
}

func rowFromReturnFlowSources(sources []effect.FlowInto) effect.Row {
	var row effect.Row
	for _, source := range sources {
		row = effect.Union(row, effect.Row{Labels: []effect.Label{source}})
	}
	return row
}

func appendAliasSourcePath(row effect.Row, suffix effect.PathSuffix) []effect.FlowInto {
	if len(row.Labels) == 0 {
		return nil
	}
	var out []effect.FlowInto
	for _, label := range row.Labels {
		flow, ok := label.(effect.FlowInto)
		if !ok {
			continue
		}
		flow.SourcePath = flow.SourcePath.Join(suffix)
		out = append(out, flow)
	}
	return out
}

func joinFlowRemainder(existing, candidate typ.Type) typ.Type {
	switch {
	case candidate == nil || typ.IsNever(candidate):
		return existing
	case existing == nil:
		return candidate
	default:
		return typjoin.Types(existing, candidate)
	}
}
