package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// WithFlow returns a BaseSynth backed by the same synthesis engine but using a
// specific flow projection. It is the canonical way for consumers to ask for a
// boundary view such as assignment pre-state without reconstructing predecessor
// joins or local caches.
func (e *Engine) WithFlow(flow api.FlowOps) api.BaseSynth {
	if e == nil || flow == nil {
		return nil
	}
	return flowSynthView{engine: e, flow: flow}
}

type flowSynthView struct {
	engine *Engine
	flow   api.FlowOps
}

func (v flowSynthView) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if v.engine == nil {
		return typ.Unknown
	}
	return v.engine.SynthExpr(expr, p, v.flow)
}

func (v flowSynthView) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if v.engine == nil {
		return typ.Unknown
	}
	if expr == nil {
		return typ.Nil
	}
	if expected == nil {
		return v.TypeOf(expr, p)
	}
	sc := v.engine.deps.Scopes[p]
	recurse := func(ex ast.Expr) typ.Type {
		return v.engine.SynthExpr(ex, p, v.flow)
	}
	return v.engine.SynthExprWithExpectedCore(expr, sc, p, recurse, expected)
}

func (v flowSynthView) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	if v.engine == nil {
		return nil
	}
	return v.engine.SynthMulti(expr, p, v.flow)
}

func (v flowSynthView) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	if v.engine == nil {
		return nil
	}
	return v.engine.FunctionType(fn, sc)
}

func (v flowSynthView) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	if v.engine == nil || len(exprs) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, needed)
	for i, expr := range exprs {
		if i == len(exprs)-1 {
			result = append(result, v.MultiTypeOf(expr, p)...)
		} else {
			result = append(result, v.TypeOf(expr, p))
		}
	}
	for len(result) < needed {
		result = append(result, typ.Nil)
	}
	if len(result) > needed {
		return result[:needed]
	}
	return result
}

func (v flowSynthView) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	if v.engine == nil {
		return nil
	}
	return v.engine.InferIterVarsWithFlow(exprs, count, p, v.flow)
}

func (v flowSynthView) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if v.engine == nil {
		return typ.Unknown
	}
	return v.engine.ResolveType(expr, sc)
}

func (v flowSynthView) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	if v.engine == nil {
		return nil
	}
	return v.engine.ResolveReturnTypes(types, sc)
}
