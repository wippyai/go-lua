package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
	bottom := product.Bottom(reg)
	for len(out.Returns) > 0 && product.Equal(reg, out.Returns[len(out.Returns)-1], bottom) {
		out.Returns = out.Returns[:len(out.Returns)-1]
	}
	top := product.Top()
	for len(out.ParamObligations) > 0 &&
		product.Equal(reg, out.ParamObligations[len(out.ParamObligations)-1], top) {
		out.ParamObligations = out.ParamObligations[:len(out.ParamObligations)-1]
	}
	out.ParamMemberCallObligations = paramMemberCallObligationLane.Normalize(out.ParamMemberCallObligations)
	out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Normalize(out.ParamMemberReturnSlots)
	out.ReturnParamPathAliases = returnParamPathAliasLane.Normalize(out.ReturnParamPathAliases)
	out.ParamSinkExposures = normalizeParamSinkExposures(reg, out.ParamSinkExposures)
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
	out.HeapTableObjects = normalizeOwnedHeapTableObjects(reg, out.HeapTableObjects)
	out.ReturnConditionParamRefinements = normalizeReturnConditionParamRefinements(
		reg,
		out.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = returnPresenceRelationLane.Normalize(out.ReturnPresenceRelations)
	if len(out.Returns) == 0 &&
		len(out.ParamObligations) == 0 &&
		len(out.ParamMemberCallObligations) == 0 &&
		len(out.ParamMemberReturnSlots) == 0 &&
		len(out.ReturnParamPathAliases) == 0 &&
		len(out.ParamSinkExposures) == 0 &&
		len(out.NormalReturnParams) == 0 &&
		len(out.NormalReturnParamConditions) == 0 &&
		len(out.NormalReturnParamEqualities) == 0 &&
		out.NormalReturnFacts.Empty() &&
		len(out.HeapTableObjects) == 0 &&
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
	return paramEqualitiesSummaryEqual(reg, a, b) &&
		paramMemberCallObligationLane.Equal(a.ParamMemberCallObligations, b.ParamMemberCallObligations) &&
		paramMemberReturnSlotLane.Equal(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots) &&
		returnParamPathAliasLane.Equal(a.ReturnParamPathAliases, b.ReturnParamPathAliases) &&
		paramSinkExposuresEqual(reg, a.ParamSinkExposures, b.ParamSinkExposures) &&
		normalReturnFactsEqual(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		heapTableObjectsEqual(reg, a.HeapTableObjects, b.HeapTableObjects) &&
		returnConditionParamRefinementsEqual(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationLane.Equal(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
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
	return paramEqualitiesSummaryLessOrEq(reg, a, b) &&
		paramMemberCallObligationLane.LessOrEq(a.ParamMemberCallObligations, b.ParamMemberCallObligations) &&
		paramMemberReturnSlotLane.LessOrEq(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots) &&
		returnParamPathAliasLane.LessOrEq(a.ReturnParamPathAliases, b.ReturnParamPathAliases) &&
		paramSinkExposuresLessOrEq(reg, a.ParamSinkExposures, b.ParamSinkExposures) &&
		normalReturnFactsLessOrEq(reg, a.NormalReturnFacts, b.NormalReturnFacts) &&
		heapTableObjectsLessOrEq(reg, a.HeapTableObjects, b.HeapTableObjects) &&
		returnConditionParamRefinementsLessOrEq(reg, a.ReturnConditionParamRefinements, b.ReturnConditionParamRefinements) &&
		returnPresenceRelationLane.LessOrEq(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
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
	if returns == 0 && obligations == 0 && params == 0 && conditions == 0 &&
		len(a.ParamMemberCallObligations) == 0 && len(b.ParamMemberCallObligations) == 0 &&
		len(a.ParamMemberReturnSlots) == 0 && len(b.ParamMemberReturnSlots) == 0 &&
		len(a.ReturnParamPathAliases) == 0 && len(b.ReturnParamPathAliases) == 0 &&
		len(a.ParamSinkExposures) == 0 && len(b.ParamSinkExposures) == 0 &&
		len(a.NormalReturnParamEqualities) == 0 && len(b.NormalReturnParamEqualities) == 0 &&
		a.NormalReturnFacts.Empty() && b.NormalReturnFacts.Empty() &&
		len(a.HeapTableObjects) == 0 && len(b.HeapTableObjects) == 0 &&
		len(a.ReturnConditionParamRefinements) == 0 && len(b.ReturnConditionParamRefinements) == 0 &&
		len(a.ReturnPresenceRelations) == 0 && len(b.ReturnPresenceRelations) == 0 {
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
	out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
		a.ParamMemberCallObligations,
		b.ParamMemberCallObligations,
	)
	out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(
		a.ParamMemberReturnSlots,
		b.ParamMemberReturnSlots,
	)
	out.ReturnParamPathAliases = returnParamPathAliasLane.Join(
		a.ReturnParamPathAliases,
		b.ReturnParamPathAliases,
	)
	out.ParamSinkExposures = joinParamSinkExposures(reg, a.ParamSinkExposures, b.ParamSinkExposures)
	out.NormalReturnParamEqualities = joinParamEqualities(reg, a, b)
	out.NormalReturnFacts = joinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
	out.HeapTableObjects = joinHeapTableObjects(reg, a.HeapTableObjects, b.HeapTableObjects)
	out.HeapKeySpace = preferHeapKeySpace(a, b)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		a.ReturnConditionParamRefinements,
		b.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = returnPresenceRelationLane.Join(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
	return NormalizeOwned(reg, out)
}

func joinReturnValue(reg *axis.Registry, left, right product.Value) product.Value {
	joined := product.Join(reg, left, right)
	return preserveJoinedReturnTypeWitness(reg, joined, left, right)
}

func widenReturnValue(reg *axis.Registry, prev, next product.Value) product.Value {
	widened := product.Widen(reg, prev, next)
	return preserveJoinedReturnTypeWitness(reg, widened, prev, next)
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
	if returns == 0 && obligations == 0 && params == 0 && conditions == 0 &&
		len(prev.ParamMemberCallObligations) == 0 && len(next.ParamMemberCallObligations) == 0 &&
		len(prev.ParamMemberReturnSlots) == 0 && len(next.ParamMemberReturnSlots) == 0 &&
		len(prev.ReturnParamPathAliases) == 0 && len(next.ReturnParamPathAliases) == 0 &&
		len(prev.ParamSinkExposures) == 0 && len(next.ParamSinkExposures) == 0 &&
		len(prev.NormalReturnParamEqualities) == 0 && len(next.NormalReturnParamEqualities) == 0 &&
		prev.NormalReturnFacts.Empty() && next.NormalReturnFacts.Empty() &&
		len(prev.HeapTableObjects) == 0 && len(next.HeapTableObjects) == 0 &&
		len(prev.ReturnConditionParamRefinements) == 0 && len(next.ReturnConditionParamRefinements) == 0 &&
		len(prev.ReturnPresenceRelations) == 0 && len(next.ReturnPresenceRelations) == 0 {
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
	out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
		prev.ParamMemberCallObligations,
		next.ParamMemberCallObligations,
	)
	out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(
		prev.ParamMemberReturnSlots,
		next.ParamMemberReturnSlots,
	)
	out.ReturnParamPathAliases = returnParamPathAliasLane.Join(
		prev.ReturnParamPathAliases,
		next.ReturnParamPathAliases,
	)
	out.ParamSinkExposures = joinParamSinkExposures(reg, prev.ParamSinkExposures, next.ParamSinkExposures)
	out.NormalReturnParamEqualities = joinParamEqualities(reg, prev, next)
	out.NormalReturnFacts = widenNormalReturnFacts(reg, prev.NormalReturnFacts, next.NormalReturnFacts)
	out.HeapTableObjects = widenHeapTableObjects(reg, prev.HeapTableObjects, next.HeapTableObjects)
	out.HeapKeySpace = preferHeapKeySpace(prev, next)
	out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
		reg,
		prev.ReturnConditionParamRefinements,
		next.ReturnConditionParamRefinements,
	)
	out.ReturnPresenceRelations = returnPresenceRelationLane.Join(prev.ReturnPresenceRelations, next.ReturnPresenceRelations)
	return NormalizeOwned(reg, out)
}

// preferHeapKeySpace selects the keyspace carried by whichever summary
// contributes heap objects. Within a single function's fixpoint both operands
// share one keyspace, so the choice is unambiguous; it is nil only when neither
// operand carries heap objects.
func preferHeapKeySpace(a, b Summary) *keyspace.KeySpace {
	if a.HeapKeySpace != nil {
		return a.HeapKeySpace
	}
	return b.HeapKeySpace
}

func summaryBottom(s Summary) bool {
	return len(s.Returns) == 0 &&
		len(s.ParamObligations) == 0 &&
		len(s.ParamMemberCallObligations) == 0 &&
		len(s.ParamMemberReturnSlots) == 0 &&
		len(s.ReturnParamPathAliases) == 0 &&
		len(s.ParamSinkExposures) == 0 &&
		len(s.NormalReturnParams) == 0 &&
		len(s.NormalReturnParamConditions) == 0 &&
		len(s.NormalReturnParamEqualities) == 0 &&
		s.NormalReturnFacts.Empty() &&
		len(s.HeapTableObjects) == 0 &&
		len(s.ReturnConditionParamRefinements) == 0 &&
		len(s.ReturnPresenceRelations) == 0
}
