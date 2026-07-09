package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Normalize returns s with trailing bottom slots removed. It defensively copies
// mutable lanes before canonicalizing, so callers can continue using s.
func Normalize(reg *axis.Registry, s Summary) Summary {
	return NormalizeOwned(reg, s.Clone())
}

// NormalizeOwned returns s with trailing bottom slots removed and may reuse or
// mutate mutable lanes in s. Callers must only use this when they own s and all
// of its map/slice lanes.
func NormalizeOwned(reg *axis.Registry, out Summary) Summary {
	for _, lane := range summaryLanes {
		lane.normalizeOwned(reg, &out)
	}
	if summaryBottom(out) {
		return Summary{}
	}
	return out
}

// NormalizedDomain returns the summary lattice for callers that own normalized
// summaries at every storage boundary. Initial values and transfer outputs must
// pass through Normalize/NormalizeOwned before entering this domain. The payoff
// is that convergence checks can compare already-canonical fact lanes without
// defensively normalizing them again.
func NormalizedDomain(reg *axis.Registry) lattice.Lattice[Summary] {
	return lattice.Lattice[Summary]{
		Bottom: func() Summary { return Summary{} },
		Equal: func(a, b Summary) bool {
			return EqualNormalized(reg, a, b)
		},
		LessOrEq: func(a, b Summary) bool {
			return LessOrEq(reg, a, b)
		},
		Join: func(a, b Summary) Summary {
			return Join(reg, a, b)
		},
		Widen: func(prev, next Summary) Summary {
			return Widen(reg, prev, next)
		},
	}
}

// Equal reports whether a and b have equal summary lanes. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Equal(reg *axis.Registry, a, b Summary) bool {
	return equal(reg, a, b, false)
}

// EqualNormalized reports whether a and b are equal when both summaries are
// already normalized. It is intended for solver-owned storage domains; use Equal
// at public boundaries or with arbitrary caller-provided summaries.
func EqualNormalized(reg *axis.Registry, a, b Summary) bool {
	return equal(reg, a, b, true)
}

func equal(reg *axis.Registry, a, b Summary, normalized bool) bool {
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.Equal(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	for i := range n {
		if !product.Equal(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.ParamObligations), len(b.ParamObligations))
	for i := range n {
		if !product.Equal(reg, paramObligationAt(reg, a, i), paramObligationAt(reg, b, i)) {
			return false
		}
	}
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if normalReturnParamConditionAt(reg, a, i) != normalReturnParamConditionAt(reg, b, i) {
			return false
		}
	}
	return summaryNonSlotLanesEqual(reg, a, b, normalized)
}

func normalReturnFactsEqualFor(reg *axis.Registry, a, b callboundary.NormalReturnFacts, normalized bool) bool {
	if normalized {
		return normalReturnFactsEqualNormalized(reg, a, b)
	}
	return normalReturnFactsEqual(reg, a, b)
}

// LessOrEq reports whether a is less than or equal to b componentwise. Missing
// return and value-constraint slots are bottom. Missing condition slots within
// the known normal-return parameter arity are top/no-constraint.
func LessOrEq(reg *axis.Registry, a, b Summary) bool {
	if summaryBottom(a) {
		return true
	}
	if summaryBottom(b) {
		return summaryBottom(a)
	}
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.LessOrEq(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	for i := range n {
		if !product.LessOrEq(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i)) {
			return false
		}
	}
	n = max(len(a.ParamObligations), len(b.ParamObligations))
	for i := range n {
		if !product.LessOrEq(reg, paramObligationAt(reg, b, i), paramObligationAt(reg, a, i)) {
			return false
		}
	}
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if !paramConditionLessOrEq(normalReturnParamConditionAt(reg, a, i), normalReturnParamConditionAt(reg, b, i)) {
			return false
		}
	}
	return summaryNonSlotLanesLessOrEq(reg, a, b)
}

// Join returns the componentwise join of a and b. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Join(reg *axis.Registry, a, b Summary) Summary {
	returns := max(len(a.Returns), len(b.Returns))
	obligations := max(len(a.ParamObligations), len(b.ParamObligations))
	params := max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	if summaryBottom(a) {
		return Normalize(reg, b)
	}
	if summaryBottom(b) {
		return Normalize(reg, a)
	}
	if returns == 0 && obligations == 0 && params == 0 && conditions == 0 && summaryPairNonSlotLanesEmpty(a, b) {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = joinReturnValue(reg, returnAt(reg, a, i), returnAt(reg, b, i))
	}
	if obligations > 0 {
		out.ParamObligations = make([]product.Value, obligations)
	}
	for i := range obligations {
		out.ParamObligations[i] = product.Meet(reg, paramObligationAt(reg, a, i), paramObligationAt(reg, b, i))
	}
	if params > 0 {
		out.NormalReturnParams = make([]product.Value, params)
	}
	for i := range params {
		out.NormalReturnParams[i] = product.Join(reg, normalReturnParamAt(reg, a, i), normalReturnParamAt(reg, b, i))
	}
	if conditions > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, conditions)
	}
	for i := range conditions {
		out.NormalReturnParamConditions[i] = joinParamCondition(
			normalReturnParamConditionAt(reg, a, i),
			normalReturnParamConditionAt(reg, b, i),
		)
	}
	assignSummaryNonSlotLanesJoin(reg, a, b, &out)
	return NormalizeOwned(reg, out)
}

// JoinReturnValue joins one function-summary return slot, preserving every
// syntactically-derived tagged-record alternative before falling back to the
// ordinary product join.
func JoinReturnValue(reg *axis.Registry, left, right product.Value) product.Value {
	joined := product.Join(reg, left, right)
	if tagged, ok := joinedTaggedReturnValue(reg, joined, left, right); ok {
		return tagged
	}
	return preserveJoinedReturnTypeWitness(reg, joined, left, right)
}

func joinReturnValue(reg *axis.Registry, left, right product.Value) product.Value {
	return JoinReturnValue(reg, left, right)
}

func widenReturnValue(reg *axis.Registry, prev, next product.Value) product.Value {
	if joined, ok := joinedReturnValueWithoutTop(reg, prev, next); ok {
		return joined
	}
	widened := product.Widen(reg, prev, next)
	return preserveJoinedReturnTypeWitness(reg, widened, prev, next)
}

func joinedReturnValueWithoutTop(reg *axis.Registry, prev, next product.Value) (product.Value, bool) {
	joined := product.Join(reg, prev, next)
	taggedJoin := false
	if tagged, ok := joinedTaggedReturnValue(reg, joined, prev, next); ok {
		joined = tagged
		taggedJoin = true
	}
	if product.Equal(reg, joined, product.Top()) {
		return product.Value{}, false
	}
	if _, ok := typevalue.TypeOf(reg, joined); !ok {
		return product.Value{}, false
	}
	if !taggedJoin && (!product.LessOrEq(reg, prev, joined) || !product.LessOrEq(reg, next, joined)) {
		return product.Value{}, false
	}
	return joined, true
}

func preserveJoinedReturnTypeWitness(reg *axis.Registry, joined, left, right product.Value) product.Value {
	if _, ok := typevalue.TypeOf(reg, joined); ok {
		return joined
	}
	leftType, leftOK := typevalue.TypeOf(reg, left)
	rightType, rightOK := typevalue.TypeOf(reg, right)
	if !leftOK || !rightOK {
		return joined
	}
	switch {
	case returnTypeIsNil(leftType) && !returnTypeIsNil(rightType):
		return typevalue.WithWitness(reg, joined, typenormalize.UnionForEvidence(typ.Nil, rightType))
	case returnTypeIsNil(rightType) && !returnTypeIsNil(leftType):
		return typevalue.WithWitness(reg, joined, typenormalize.UnionForEvidence(leftType, typ.Nil))
	default:
		return joined
	}
}

func returnTypeIsNil(t typ.Type) bool {
	return t != nil && t.Kind() == kind.Nil
}

// Widen returns the componentwise widening from prev to next. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Widen(reg *axis.Registry, prev, next Summary) Summary {
	returns := max(len(prev.Returns), len(next.Returns))
	obligations := max(len(prev.ParamObligations), len(next.ParamObligations))
	params := max(len(prev.NormalReturnParams), len(next.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, prev), normalReturnParamCount(reg, next))
	if summaryBottom(prev) {
		return Normalize(reg, next)
	}
	if summaryBottom(next) {
		return Normalize(reg, prev)
	}
	if returns == 0 && obligations == 0 && params == 0 && conditions == 0 && summaryPairNonSlotLanesEmpty(prev, next) {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = widenReturnValue(reg, returnAt(reg, prev, i), returnAt(reg, next, i))
	}
	if obligations > 0 {
		out.ParamObligations = make([]product.Value, obligations)
	}
	for i := range obligations {
		out.ParamObligations[i] = product.Meet(
			reg,
			paramObligationAt(reg, prev, i),
			paramObligationAt(reg, next, i),
		)
	}
	if params > 0 {
		out.NormalReturnParams = make([]product.Value, params)
	}
	for i := range params {
		out.NormalReturnParams[i] = product.Widen(
			reg,
			normalReturnParamAt(reg, prev, i),
			normalReturnParamAt(reg, next, i),
		)
	}
	if conditions > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, conditions)
	}
	for i := range conditions {
		out.NormalReturnParamConditions[i] = widenParamCondition(
			normalReturnParamConditionAt(reg, prev, i),
			normalReturnParamConditionAt(reg, next, i),
		)
	}
	assignSummaryNonSlotLanesWiden(reg, prev, next, &out)
	return NormalizeOwned(reg, out)
}

func summaryBottom(s Summary) bool {
	return summaryLanesEmpty(s)
}

func summaryPairNonSlotLanesEmpty(a, b Summary) bool {
	return summaryNonSlotLanesEmpty(a) && summaryNonSlotLanesEmpty(b)
}
