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
