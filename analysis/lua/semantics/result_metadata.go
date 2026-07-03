package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
)

func (r *Result) TypeDefinition(point cfg.Point) (cfgfacts.TypeDefinitionFact, bool) {
	if r == nil {
		return cfgfacts.TypeDefinitionFact{}, false
	}
	return r.meta.TypeDefinition(point)
}

func (r *Result) FunctionDefinition(point cfg.Point) (cfgfacts.FunctionDefinitionFact, bool) {
	if r == nil {
		return cfgfacts.FunctionDefinitionFact{}, false
	}
	return r.meta.FunctionDefinition(point)
}

func (r *Result) NumericFor(point cfg.Point) (cfgfacts.NumericForFact, bool) {
	if r == nil {
		return cfgfacts.NumericForFact{}, false
	}
	return r.meta.NumericFor(point)
}

func (r *Result) GenericFor(point cfg.Point) (cfgfacts.GenericForFact, bool) {
	if r == nil {
		return cfgfacts.GenericForFact{}, false
	}
	return r.meta.GenericFor(point)
}

func (r *Result) Label(point cfg.Point) (cfgfacts.LabelFact, bool) {
	if r == nil {
		return cfgfacts.LabelFact{}, false
	}
	return r.meta.Label(point)
}

func (r *Result) Goto(point cfg.Point) (cfgfacts.GotoFact, bool) {
	if r == nil {
		return cfgfacts.GotoFact{}, false
	}
	return r.meta.Goto(point)
}

func (r *Result) ExpressionEvaluation(point cfg.Point) (cfgfacts.ExpressionEvaluationFact, bool) {
	if r == nil {
		return cfgfacts.ExpressionEvaluationFact{}, false
	}
	return r.meta.ExpressionEvaluation(point)
}
