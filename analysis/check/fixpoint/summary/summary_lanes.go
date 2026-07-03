package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type summaryLane struct {
	fieldName      string
	slot           bool
	empty          func(Summary) bool
	assignClone    func(src Summary, dst *Summary)
	normalizeOwned func(reg *axis.Registry, s *Summary)
	equal          func(reg *axis.Registry, a, b Summary, normalized bool) bool
	lessOrEq       func(reg *axis.Registry, a, b Summary) bool
	assignJoin     func(reg *axis.Registry, a, b Summary, out *Summary)
	assignWiden    func(reg *axis.Registry, prev, next Summary, out *Summary)
}

var summaryLanes = []summaryLane{
	{
		fieldName:   "Returns",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.Returns) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.Returns = cloneSlice(src.Returns) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.Returns = trimTrailingProducts(reg, s.Returns, product.Bottom(reg))
		},
	},
	{
		fieldName:   "ParamObligations",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.ParamObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamObligations = cloneSlice(src.ParamObligations) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ParamObligations = trimTrailingProducts(reg, s.ParamObligations, product.Top())
		},
	},
	{
		fieldName: "ParamMemberCallObligations",
		empty:     func(s Summary) bool { return len(s.ParamMemberCallObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ParamMemberCallObligations = cloneSlice(src.ParamMemberCallObligations)
		},
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ParamMemberCallObligations = paramMemberCallObligationLane.Normalize(s.ParamMemberCallObligations)
		},
		equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
			return paramMemberCallObligationLane.Equal(a.ParamMemberCallObligations, b.ParamMemberCallObligations)
		},
		lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
			return paramMemberCallObligationLane.LessOrEq(a.ParamMemberCallObligations, b.ParamMemberCallObligations)
		},
		assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
			out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
				a.ParamMemberCallObligations,
				b.ParamMemberCallObligations,
			)
		},
		assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
			out.ParamMemberCallObligations = paramMemberCallObligationLane.Join(
				prev.ParamMemberCallObligations,
				next.ParamMemberCallObligations,
			)
		},
	},
	{
		fieldName:   "ParamMemberReturnSlots",
		empty:       func(s Summary) bool { return len(s.ParamMemberReturnSlots) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamMemberReturnSlots = cloneSlice(src.ParamMemberReturnSlots) },
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ParamMemberReturnSlots = paramMemberReturnSlotLane.Normalize(s.ParamMemberReturnSlots)
		},
		equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
			return paramMemberReturnSlotLane.Equal(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
		},
		lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
			return paramMemberReturnSlotLane.LessOrEq(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
		},
		assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
			out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(a.ParamMemberReturnSlots, b.ParamMemberReturnSlots)
		},
		assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
			out.ParamMemberReturnSlots = paramMemberReturnSlotLane.Join(prev.ParamMemberReturnSlots, next.ParamMemberReturnSlots)
		},
	},
	{
		fieldName:   "ReturnParamPathAliases",
		empty:       func(s Summary) bool { return len(s.ReturnParamPathAliases) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ReturnParamPathAliases = cloneSlice(src.ReturnParamPathAliases) },
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ReturnParamPathAliases = returnParamPathAliasLane.Normalize(s.ReturnParamPathAliases)
		},
		equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
			return returnParamPathAliasLane.Equal(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
		},
		lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
			return returnParamPathAliasLane.LessOrEq(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
		},
		assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
			out.ReturnParamPathAliases = returnParamPathAliasLane.Join(a.ReturnParamPathAliases, b.ReturnParamPathAliases)
		},
		assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
			out.ReturnParamPathAliases = returnParamPathAliasLane.Join(prev.ReturnParamPathAliases, next.ReturnParamPathAliases)
		},
	},
	{
		fieldName:   "ParamSinkExposures",
		empty:       func(s Summary) bool { return len(s.ParamSinkExposures) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamSinkExposures = cloneSlice(src.ParamSinkExposures) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ParamSinkExposures = normalizeParamSinkExposures(reg, s.ParamSinkExposures)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return paramSinkExposuresEqual(reg, a.ParamSinkExposures, b.ParamSinkExposures)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return paramSinkExposuresLessOrEq(reg, a.ParamSinkExposures, b.ParamSinkExposures)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.ParamSinkExposures = joinParamSinkExposures(reg, a.ParamSinkExposures, b.ParamSinkExposures)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.ParamSinkExposures = joinParamSinkExposures(reg, prev.ParamSinkExposures, next.ParamSinkExposures)
		},
	},
	{
		fieldName:   "CapturedPathObligations",
		empty:       func(s Summary) bool { return len(s.CapturedPathObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.CapturedPathObligations = cloneSlice(src.CapturedPathObligations) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.CapturedPathObligations = normalizeCapturedPathObligations(reg, s.CapturedPathObligations)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return capturedPathObligationsEqual(reg, a.CapturedPathObligations, b.CapturedPathObligations)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return capturedPathObligationsLessOrEq(reg, a.CapturedPathObligations, b.CapturedPathObligations)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.CapturedPathObligations = joinCapturedPathObligations(
				reg,
				a.CapturedPathObligations,
				b.CapturedPathObligations,
			)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.CapturedPathObligations = widenCapturedPathObligations(
				reg,
				prev.CapturedPathObligations,
				next.CapturedPathObligations,
			)
		},
	},
	{
		fieldName:   "NormalReturnParams",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.NormalReturnParams) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.NormalReturnParams = cloneSlice(src.NormalReturnParams) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.NormalReturnParams = trimTrailingProducts(reg, s.NormalReturnParams, product.Bottom(reg))
		},
	},
	{
		fieldName: "NormalReturnParamConditions",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.NormalReturnParamConditions) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.NormalReturnParamConditions = cloneSlice(src.NormalReturnParamConditions)
		},
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			for len(s.NormalReturnParamConditions) > 0 &&
				!s.NormalReturnParamConditions[len(s.NormalReturnParamConditions)-1].IsUseful() {
				s.NormalReturnParamConditions = s.NormalReturnParamConditions[:len(s.NormalReturnParamConditions)-1]
			}
		},
	},
	{
		fieldName: "NormalReturnParamEqualities",
		empty:     func(s Summary) bool { return len(s.NormalReturnParamEqualities) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.NormalReturnParamEqualities = cloneSlice(src.NormalReturnParamEqualities)
		},
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.NormalReturnParamEqualities = normalizeParamEqualities(s.NormalReturnParamEqualities)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return paramEqualitiesSummaryEqual(reg, a, b)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return paramEqualitiesSummaryLessOrEq(reg, a, b)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.NormalReturnParamEqualities = joinParamEqualities(reg, a, b)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.NormalReturnParamEqualities = joinParamEqualities(reg, prev, next)
		},
	},
	{
		fieldName:   "NormalReturnFacts",
		empty:       func(s Summary) bool { return s.NormalReturnFacts.Empty() },
		assignClone: func(src Summary, dst *Summary) { dst.NormalReturnFacts = cloneNormalReturnFacts(src.NormalReturnFacts) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.NormalReturnFacts = normalizeOwnedNormalReturnFacts(reg, s.NormalReturnFacts)
		},
		equal: func(reg *axis.Registry, a, b Summary, normalized bool) bool {
			return normalReturnFactsEqualFor(reg, a.NormalReturnFacts, b.NormalReturnFacts, normalized)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return normalReturnFactsLessOrEq(reg, a.NormalReturnFacts, b.NormalReturnFacts)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.NormalReturnFacts = joinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.NormalReturnFacts = widenNormalReturnFacts(reg, prev.NormalReturnFacts, next.NormalReturnFacts)
		},
	},
	{
		fieldName:   "HeapTableObjects",
		empty:       func(s Summary) bool { return len(s.HeapTableObjects) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.HeapTableObjects = cloneHeapTableObjects(src.HeapTableObjects) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.HeapTableObjects = normalizeOwnedHeapTableObjects(reg, s.HeapTableObjects)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return heapTableObjectsEqual(reg, a.HeapTableObjects, b.HeapTableObjects)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return heapTableObjectsLessOrEq(reg, a.HeapTableObjects, b.HeapTableObjects)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.HeapTableObjects, out.HeapKeySpace = joinSummaryHeapTableObjects(reg, a, b)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.HeapTableObjects, out.HeapKeySpace = widenSummaryHeapTableObjects(reg, prev, next)
		},
	},
	{
		fieldName: "ReturnConditionParamRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionParamRefinements) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnConditionParamRefinements = cloneReturnConditionParamRefinements(src.ReturnConditionParamRefinements)
		},
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ReturnConditionParamRefinements = normalizeReturnConditionParamRefinements(
				reg,
				s.ReturnConditionParamRefinements,
			)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return returnConditionParamRefinementsEqual(
				reg,
				a.ReturnConditionParamRefinements,
				b.ReturnConditionParamRefinements,
			)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return returnConditionParamRefinementsLessOrEq(
				reg,
				a.ReturnConditionParamRefinements,
				b.ReturnConditionParamRefinements,
			)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
				reg,
				a.ReturnConditionParamRefinements,
				b.ReturnConditionParamRefinements,
			)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.ReturnConditionParamRefinements = joinReturnConditionParamRefinements(
				reg,
				prev.ReturnConditionParamRefinements,
				next.ReturnConditionParamRefinements,
			)
		},
	},
	{
		fieldName: "ReturnConditionSlotRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionSlotRefinements) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnConditionSlotRefinements = cloneReturnConditionSlotRefinements(src.ReturnConditionSlotRefinements)
		},
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ReturnConditionSlotRefinements = normalizeReturnConditionSlotRefinements(
				reg,
				s.ReturnConditionSlotRefinements,
			)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return returnConditionSlotRefinementsEqual(
				reg,
				a.ReturnConditionSlotRefinements,
				b.ReturnConditionSlotRefinements,
			)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return returnConditionSlotRefinementsLessOrEq(
				reg,
				a.ReturnConditionSlotRefinements,
				b.ReturnConditionSlotRefinements,
			)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.ReturnConditionSlotRefinements = joinReturnConditionSlotRefinements(
				reg,
				a.ReturnConditionSlotRefinements,
				b.ReturnConditionSlotRefinements,
			)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.ReturnConditionSlotRefinements = joinReturnConditionSlotRefinements(
				reg,
				prev.ReturnConditionSlotRefinements,
				next.ReturnConditionSlotRefinements,
			)
		},
	},
	{
		fieldName: "ReturnParamLiteralCases",
		empty:     func(s Summary) bool { return len(s.ReturnParamLiteralCases) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnParamLiteralCases = cloneReturnParamLiteralCases(src.ReturnParamLiteralCases)
		},
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ReturnParamLiteralCases = normalizeReturnParamLiteralCases(reg, s.ReturnParamLiteralCases)
		},
		equal: func(reg *axis.Registry, a, b Summary, _ bool) bool {
			return returnParamLiteralCasesEqual(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
		},
		lessOrEq: func(reg *axis.Registry, a, b Summary) bool {
			return returnParamLiteralCasesLessOrEq(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
		},
		assignJoin: func(reg *axis.Registry, a, b Summary, out *Summary) {
			out.ReturnParamLiteralCases = joinReturnParamLiteralCases(reg, a.ReturnParamLiteralCases, b.ReturnParamLiteralCases)
		},
		assignWiden: func(reg *axis.Registry, prev, next Summary, out *Summary) {
			out.ReturnParamLiteralCases = widenReturnParamLiteralCases(
				reg,
				prev.ReturnParamLiteralCases,
				next.ReturnParamLiteralCases,
			)
		},
	},
	{
		fieldName: "ReturnPresenceRelations",
		empty:     func(s Summary) bool { return len(s.ReturnPresenceRelations) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnPresenceRelations = returnPresenceRelationLane.Clone(src.ReturnPresenceRelations)
		},
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ReturnPresenceRelations = returnPresenceRelationLane.Normalize(s.ReturnPresenceRelations)
		},
		equal: func(_ *axis.Registry, a, b Summary, _ bool) bool {
			return returnPresenceRelationLane.Equal(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
		},
		lessOrEq: func(_ *axis.Registry, a, b Summary) bool {
			return returnPresenceRelationLane.LessOrEq(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
		},
		assignJoin: func(_ *axis.Registry, a, b Summary, out *Summary) {
			out.ReturnPresenceRelations = returnPresenceRelationLane.Join(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
		},
		assignWiden: func(_ *axis.Registry, prev, next Summary, out *Summary) {
			out.ReturnPresenceRelations = returnPresenceRelationLane.Join(
				prev.ReturnPresenceRelations,
				next.ReturnPresenceRelations,
			)
		},
	},
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func trimTrailingProducts(reg *axis.Registry, in []product.Value, trailing product.Value) []product.Value {
	for len(in) > 0 && product.Equal(reg, in[len(in)-1], trailing) {
		in = in[:len(in)-1]
	}
	return in
}

func summaryLanesEmpty(s Summary) bool {
	for _, lane := range summaryLanes {
		if !lane.empty(s) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesEmpty(s Summary) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.empty(s) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesEqual(reg *axis.Registry, a, b Summary, normalized bool) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.equal(reg, a, b, normalized) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesLessOrEq(reg *axis.Registry, a, b Summary) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.lessOrEq(reg, a, b) {
			return false
		}
	}
	return true
}

func assignSummaryNonSlotLanesJoin(reg *axis.Registry, a, b Summary, out *Summary) {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		lane.assignJoin(reg, a, b, out)
	}
}

func assignSummaryNonSlotLanesWiden(reg *axis.Registry, prev, next Summary, out *Summary) {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		lane.assignWiden(reg, prev, next, out)
	}
}
