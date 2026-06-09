package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
)

// Empty reports whether a postflow projection product carries no compatibility
// evidence.
func ProjectionProductEmpty(f ProjectionProduct) bool {
	return len(f.FunctionFacts) == 0 &&
		len(f.CapturedTypes) == 0 &&
		len(f.CapturedFields) == 0 &&
		len(f.ConstructorFields) == 0
}

// ProjectionProductMapEqual compares graph-keyed postflow projection products.
func ProjectionProductMapEqual(a, b map[api.GraphKey]ProjectionProduct) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !ProjectionProductEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenProjectionProductMap merges a next iteration fact map into the stable projection product
// using the product widening policy for each graph key.
func WidenProjectionProductMap(prev, next map[api.GraphKey]ProjectionProduct) map[api.GraphKey]ProjectionProduct {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]ProjectionProduct)
	}
	out := make(map[api.GraphKey]ProjectionProduct, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenProjectionProduct(existing, next[key])
		} else {
			out[key] = WidenProjectionProduct(ProjectionProduct{}, next[key])
		}
	}
	return out
}

// OverlayProjectionProduct returns projection facts visible during an iteration from a stable
// previous product and same-iteration next facts. The visible product crosses an
// SCC iteration boundary, so it uses the same finite-height convergence law as
// the boundary swap rather than the precise delta join.
func OverlayProjectionProduct(prev, next ProjectionProduct) ProjectionProduct {
	switch {
	case ProjectionProductEmpty(prev):
		return next
	case ProjectionProductEmpty(next):
		return prev
	default:
		return WidenProjectionProduct(prev, next)
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
func ConstructorFieldMapEqual(sym cfg.SymbolID, a, b FieldValues) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return ConstructorFieldsEqual(
		api.ConstructorFields{sym: a},
		api.ConstructorFields{sym: b},
	)
}
