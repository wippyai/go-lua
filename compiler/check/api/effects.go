package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// RefinementFacts provides function refinement lookup.
type RefinementFacts interface {
	LookupBySym(sym cfg.SymbolID) *constraint.FunctionRefinement
}

// RefinementLookup adapts FunctionFacts projections into refinement
// facts used by flow interpretation and call-effect propagation.
type RefinementLookup func(sym cfg.SymbolID) *constraint.FunctionRefinement

// NewRefinementFacts creates RefinementFacts from a projection lookup function.
func NewRefinementFacts(lookup RefinementLookup) RefinementFacts {
	if lookup == nil {
		return nilRefinementFacts{}
	}
	return lookup
}

func (f RefinementLookup) LookupBySym(sym cfg.SymbolID) *constraint.FunctionRefinement {
	if f == nil || sym == 0 {
		return nil
	}
	return f(sym)
}

// nilRefinementFacts is a no-op RefinementFacts implementation.
type nilRefinementFacts struct{}

func (nilRefinementFacts) LookupBySym(cfg.SymbolID) *constraint.FunctionRefinement { return nil }
