package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/typ"
)

// ObservedSummary reduces reachable return statements into the function's
// post-flow return vector. It consumes the solved abstract-interpreter surface;
// callers must not re-run local function body inference to publish summaries.
func ObservedSummary(graph *cfg.Graph, returns []api.ReturnEvidence, flow api.FlowOps, synth ExprSynth) []typ.Type {
	if len(returns) == 0 {
		return nil
	}
	var summary []typ.Type
	seen := false
	for _, ret := range returns {
		p := ret.Point
		if flow != nil && flow.IsPointDead(p) {
			continue
		}
		if ret.Info == nil {
			continue
		}
		vector := ReturnVector(graph, ret.Info.Exprs, p, synth)
		if !seen {
			seen = true
			summary = vector
			continue
		}
		summary = returnsummary.Merge(summary, vector)
	}
	return summary
}

// ExprSynth is the expression surface needed to expand one Lua return list.
type ExprSynth interface {
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type
	MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type
}

// ExpandValues expands a return/call expression list to the requested slot
// count using the same Lua multi-return rule as ReturnVector, padding missing
// slots with nil. This is the canonical return-list expansion surface for
// post-flow fact classifiers.
func ExpandValues(exprs []ast.Expr, needed int, p cfg.Point, synth ExprSynth) []typ.Type {
	if needed <= 0 {
		return nil
	}
	values := make([]typ.Type, 0, needed)
	for i, expr := range exprs {
		var expanded []typ.Type
		if i == len(exprs)-1 && ast.CanProduceMultipleValues(expr) {
			if synth != nil {
				expanded = synth.MultiTypeOf(expr, p)
			}
		} else if synth != nil {
			expanded = []typ.Type{synth.TypeOf(expr, p)}
		}
		if len(expanded) == 0 {
			expanded = []typ.Type{typ.Unknown}
		}
		for _, value := range expanded {
			if value == nil {
				value = typ.Unknown
			}
			values = append(values, value)
			if len(values) == needed {
				return values
			}
		}
	}
	for len(values) < needed {
		values = append(values, typ.Nil)
	}
	return values
}

// ReturnVector expands one syntactic return statement into the canonical return
// vector. A bare `return` is a real one-slot nil outcome; only absence of return
// evidence means "no summary".
func ReturnVector(graph *cfg.Graph, exprs []ast.Expr, p cfg.Point, synth ExprSynth) []typ.Type {
	if len(exprs) == 0 {
		return []typ.Type{typ.Nil}
	}
	out := make([]typ.Type, 0, len(exprs))
	for i, expr := range exprs {
		if IsImplicitSelfReturn(graph, expr) {
			out = append(out, typ.Self)
			continue
		}
		if i == len(exprs)-1 && ast.CanProduceMultipleValues(expr) {
			values := []typ.Type{typ.Unknown}
			if synth != nil {
				values = synth.MultiTypeOf(expr, p)
			}
			if len(values) == 0 {
				out = append(out, typ.Unknown)
				continue
			}
			for _, value := range values {
				if value == nil {
					value = typ.Unknown
				}
				out = append(out, value)
			}
			continue
		}
		value := typ.Unknown
		if synth != nil {
			value = synth.TypeOf(expr, p)
		}
		if value == nil {
			value = typ.Unknown
		}
		out = append(out, value)
	}
	return out
}

// IsImplicitSelfReturn reports whether expr is the implicit receiver parameter
// of a colon-defined method. Such returns remain polymorphic as Self until the
// call projection substitutes the actual receiver.
func IsImplicitSelfReturn(graph *cfg.Graph, expr ast.Expr) bool {
	if graph == nil || expr == nil {
		return false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || ident.Value != "self" {
		return false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return false
	}
	for _, slot := range graph.ParamSlotsReadOnly() {
		if slot.IsImplicitSelf && slot.Symbol == sym {
			return true
		}
	}
	return false
}
