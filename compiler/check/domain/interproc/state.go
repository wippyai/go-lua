package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// Empty reports whether a canonical interprocedural fact product carries no
// semantic evidence.
func Empty(f api.Facts) bool {
	return len(f.FunctionFacts) == 0 &&
		len(f.LiteralSigs) == 0 &&
		len(f.CapturedTypes) == 0 &&
		len(f.CapturedFields) == 0 &&
		len(f.CapturedContainers) == 0 &&
		len(f.ConstructorFields) == 0
}

// FactMapEqual compares graph-keyed interprocedural fact products.
func FactMapEqual(a, b map[api.GraphKey]api.Facts) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !FactsEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenFactMap merges a next iteration fact map into the stable product using
// the interprocedural widening policy for each graph key.
func WidenFactMap(prev, next map[api.GraphKey]api.Facts) map[api.GraphKey]api.Facts {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]api.Facts)
	}
	out := make(map[api.GraphKey]api.Facts, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenFacts(existing, next[key])
		} else {
			out[key] = WidenFacts(api.Facts{}, next[key])
		}
	}
	return out
}

// OverlayFacts returns the canonical facts visible during an iteration from a
// stable previous product and same-iteration next facts. The visible product
// crosses an SCC iteration boundary, so it uses the same finite-height
// convergence law as the boundary swap rather than the precise delta join.
func OverlayFacts(prev, next api.Facts) api.Facts {
	switch {
	case Empty(prev):
		return next
	case Empty(next):
		return prev
	default:
		return WidenFacts(prev, next)
	}
}

// RefinementEqual compares two refinement summaries.
func RefinementEqual(a, b *constraint.FunctionRefinement) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equals(b)
}

// ConstructorFieldMapEqual compares one class-symbol constructor field map.
func ConstructorFieldMapEqual(sym cfg.SymbolID, a, b map[string]product.AbstractValue) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return ConstructorFieldsEqual(
		api.ConstructorFields{sym: a},
		api.ConstructorFields{sym: b},
	)
}
