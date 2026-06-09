package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// Equal checks one FunctionFacts projection slot. The vector carriers are
// product.AbstractValue rows, so equality follows the product-domain relation
// instead of projected typ.Type equality.
func Equal(a, b api.FunctionFact) bool {
	if !product.EqualVector(a.Call.Params, b.Call.Params) {
		return false
	}
	if !product.EqualVector(a.Body.Params, b.Body.Params) {
		return false
	}
	if !product.EqualVector(a.Entry.Params, b.Entry.Params) {
		return false
	}
	if !product.EqualVector(a.Returns.Preflow, b.Returns.Preflow) {
		return false
	}
	if !product.EqualVector(a.Returns.Narrow, b.Returns.Narrow) {
		return false
	}
	if !value.FactTypeEqual(a.Public.Signature, b.Public.Signature) {
		return false
	}
	if !refinementEqual(a.Effects.Refinement, b.Effects.Refinement) {
		return false
	}
	if !EnvReturnsEqual(a.Export.EnvReturns, b.Export.EnvReturns) {
		return false
	}
	return true
}

// FactsEqual checks whether two per-symbol FunctionFacts projections are equal.
func FactsEqual(a, b api.FunctionFacts) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		af := a[sym]
		bf, ok := b[sym]
		if !ok || !Equal(af, bf) {
			return false
		}
	}
	return true
}

func refinementEqual(a, b *constraint.FunctionRefinement) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}
