package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Normalize returns s with trailing bottom slots removed.
func Normalize(reg *axis.Registry, s Summary) Summary {
	out := s.Clone()
	bottom := product.Bottom(reg)
	for len(out.Returns) > 0 && product.Equal(reg, out.Returns[len(out.Returns)-1], bottom) {
		out.Returns = out.Returns[:len(out.Returns)-1]
	}
	for len(out.NormalReturnParams) > 0 &&
		product.Equal(reg, out.NormalReturnParams[len(out.NormalReturnParams)-1], bottom) {
		out.NormalReturnParams = out.NormalReturnParams[:len(out.NormalReturnParams)-1]
	}
	for len(out.NormalReturnParamConditions) > 0 &&
		!out.NormalReturnParamConditions[len(out.NormalReturnParamConditions)-1].IsUseful() {
		out.NormalReturnParamConditions = out.NormalReturnParamConditions[:len(out.NormalReturnParamConditions)-1]
	}
	out.NormalReturnParamEqualities = normalizeParamEqualities(out.NormalReturnParamEqualities)
	out.NormalReturnFacts = normalizeNormalReturnFacts(reg, out.NormalReturnFacts)
	out.ReturnConditionParamRefinements = normalizeReturnConditionParamRefinements(
		reg,
		out.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = normalizeReturnPresenceRelations(out.ReturnPresenceRelations)
	if len(out.Returns) == 0 &&
		len(out.NormalReturnParams) == 0 &&
		len(out.NormalReturnParamConditions) == 0 &&
		len(out.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(out.NormalReturnFacts) &&
		len(out.ReturnConditionParamRefinements) == 0 &&
		len(out.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	return out
}

// Equal reports whether a and b have equal summary lanes. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Equal(reg *axis.Registry, a, b Summary) bool {
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
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if normalReturnParamConditionAt(reg, a, i) != normalReturnParamConditionAt(reg, b, i) {
			return false
		}
	}
	return paramEqualitiesSummaryEqual(reg, a, b) &&
		normalReturnFactsEqual(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		returnConditionParamRefinementsEqual(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationsEqual(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
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
	n = max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	for i := range n {
		if !paramConditionLessOrEq(normalReturnParamConditionAt(reg, a, i), normalReturnParamConditionAt(reg, b, i)) {
			return false
		}
	}
	return paramEqualitiesSummaryLessOrEq(reg, a, b) &&
		normalReturnFactsLessOrEq(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		returnConditionParamRefinementsLessOrEq(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationsLessOrEq(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
}

// Join returns the componentwise join of a and b. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Join(reg *axis.Registry, a, b Summary) Summary {
	returns := max(len(a.Returns), len(b.Returns))
	params := max(len(a.NormalReturnParams), len(b.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, a), normalReturnParamCount(reg, b))
	if summaryBottom(a) {
		return Normalize(reg, b)
	}
	if summaryBottom(b) {
		return Normalize(reg, a)
	}
	if returns == 0 && params == 0 && conditions == 0 &&
		len(a.NormalReturnParamEqualities) == 0 && len(b.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(a.NormalReturnFacts) && normalReturnFactsEmpty(b.NormalReturnFacts) &&
		len(a.ReturnConditionParamRefinements) == 0 && len(b.ReturnConditionParamRefinements) == 0 &&
		len(a.ReturnPresenceRelations) == 0 && len(b.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = product.Join(reg, returnAt(reg, a, i), returnAt(reg, b, i))
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
	out.NormalReturnParamEqualities = joinParamEqualities(reg, a, b)
	out.NormalReturnFacts = joinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		a.ReturnConditionParamRefinements,
		b.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = joinReturnPresenceRelations(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
	return Normalize(reg, out)
}

// Widen returns the componentwise widening from prev to next. Missing return and
// value-constraint slots are bottom. Missing condition slots within the known
// normal-return parameter arity are top/no-constraint.
func Widen(reg *axis.Registry, prev, next Summary) Summary {
	returns := max(len(prev.Returns), len(next.Returns))
	params := max(len(prev.NormalReturnParams), len(next.NormalReturnParams))
	conditions := max(normalReturnParamCount(reg, prev), normalReturnParamCount(reg, next))
	if summaryBottom(prev) {
		return Normalize(reg, next)
	}
	if summaryBottom(next) {
		return Normalize(reg, prev)
	}
	if returns == 0 && params == 0 && conditions == 0 &&
		len(prev.NormalReturnParamEqualities) == 0 && len(next.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(prev.NormalReturnFacts) && normalReturnFactsEmpty(next.NormalReturnFacts) &&
		len(prev.ReturnConditionParamRefinements) == 0 && len(next.ReturnConditionParamRefinements) == 0 &&
		len(prev.ReturnPresenceRelations) == 0 && len(next.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if returns > 0 {
		out.Returns = make([]product.Value, returns)
	}
	for i := range returns {
		out.Returns[i] = product.Widen(reg, returnAt(reg, prev, i), returnAt(reg, next, i))
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
	out.NormalReturnParamEqualities = joinParamEqualities(reg, prev, next)
	out.NormalReturnFacts = widenNormalReturnFacts(reg, prev.NormalReturnFacts, next.NormalReturnFacts)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		prev.ReturnConditionParamRefinements,
		next.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = joinReturnPresenceRelations(prev.ReturnPresenceRelations, next.ReturnPresenceRelations)
	return Normalize(reg, out)
}

func summaryBottom(s Summary) bool {
	return len(s.Returns) == 0 &&
		len(s.NormalReturnParams) == 0 &&
		len(s.NormalReturnParamConditions) == 0 &&
		len(s.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(s.NormalReturnFacts) &&
		len(s.ReturnConditionParamRefinements) == 0 &&
		len(s.ReturnPresenceRelations) == 0
}
