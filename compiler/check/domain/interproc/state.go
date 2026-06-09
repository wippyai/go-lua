package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
	"github.com/wippyai/go-lua/types/constraint"
)

// FunctionFactMapsEqual compares graph-keyed function-fact projection lanes.
func FunctionFactMapsEqual(a, b map[api.GraphKey]api.FunctionFacts) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !FunctionFactsEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenFunctionFactMaps merges next-iteration function-fact projections into
// the stable lane.
func WidenFunctionFactMaps(prev, next map[api.GraphKey]api.FunctionFacts) map[api.GraphKey]api.FunctionFacts {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]api.FunctionFacts)
	}
	out := make(map[api.GraphKey]api.FunctionFacts, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenFunctionFacts(existing, next[key])
		} else {
			out[key] = WidenFunctionFacts(nil, next[key])
		}
	}
	return out
}

// OverlayFunctionFacts returns projection facts visible during an iteration.
// The visible lane crosses an SCC iteration boundary, so it uses the same
// finite-height convergence law as the boundary swap.
func OverlayFunctionFacts(prev, next api.FunctionFacts) api.FunctionFacts {
	switch {
	case len(prev) == 0:
		return next
	case len(next) == 0:
		return prev
	default:
		return WidenFunctionFacts(prev, next)
	}
}

// CapturedTypeMapsEqual compares graph-keyed captured-type projection lanes.
func CapturedTypeMapsEqual(a, b map[api.GraphKey]postflow.CapturedTypes) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !symbolTypeMapEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenCapturedTypeMaps merges next-iteration captured-type projections into
// the stable lane.
func WidenCapturedTypeMaps(prev, next map[api.GraphKey]postflow.CapturedTypes) map[api.GraphKey]postflow.CapturedTypes {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]postflow.CapturedTypes)
	}
	out := make(map[api.GraphKey]postflow.CapturedTypes, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenCapturedTypes(existing, next[key])
		} else {
			out[key] = WidenCapturedTypes(nil, next[key])
		}
	}
	return out
}

// CapturedFieldAssignMapsEqual compares graph-keyed captured-field projection lanes.
func CapturedFieldAssignMapsEqual(a, b map[api.GraphKey]postflow.CapturedFieldAssigns) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !CapturedFieldAssignsEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenCapturedFieldAssignMaps merges next-iteration captured-field projections
// into the stable lane.
func WidenCapturedFieldAssignMaps(prev, next map[api.GraphKey]postflow.CapturedFieldAssigns) map[api.GraphKey]postflow.CapturedFieldAssigns {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]postflow.CapturedFieldAssigns)
	}
	out := make(map[api.GraphKey]postflow.CapturedFieldAssigns, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenCapturedFieldAssigns(existing, next[key])
		} else {
			out[key] = WidenCapturedFieldAssigns(nil, next[key])
		}
	}
	return out
}

// ConstructorFieldMapsEqual compares graph-keyed constructor-field projection lanes.
func ConstructorFieldMapsEqual(a, b map[api.GraphKey]postflow.ConstructorFields) bool {
	if len(a) != len(b) {
		return false
	}
	for _, key := range api.SortedGraphKeys(a) {
		if !ConstructorFieldsEqual(a[key], b[key]) {
			return false
		}
	}
	return true
}

// WidenConstructorFieldMaps merges next-iteration constructor-field projections
// into the stable lane.
func WidenConstructorFieldMaps(prev, next map[api.GraphKey]postflow.ConstructorFields) map[api.GraphKey]postflow.ConstructorFields {
	if len(prev) == 0 && len(next) == 0 {
		return make(map[api.GraphKey]postflow.ConstructorFields)
	}
	out := make(map[api.GraphKey]postflow.ConstructorFields, len(prev)+len(next))
	for _, key := range api.SortedGraphKeys(prev) {
		out[key] = prev[key]
	}
	for _, key := range api.SortedGraphKeys(next) {
		if existing, ok := out[key]; ok {
			out[key] = WidenConstructorFields(existing, next[key])
		} else {
			out[key] = WidenConstructorFields(nil, next[key])
		}
	}
	return out
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
		postflow.ConstructorFields{sym: a},
		postflow.ConstructorFields{sym: b},
	)
}
