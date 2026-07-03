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
	},
	{
		fieldName:   "ParamMemberReturnSlots",
		empty:       func(s Summary) bool { return len(s.ParamMemberReturnSlots) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamMemberReturnSlots = cloneSlice(src.ParamMemberReturnSlots) },
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ParamMemberReturnSlots = paramMemberReturnSlotLane.Normalize(s.ParamMemberReturnSlots)
		},
	},
	{
		fieldName:   "ReturnParamPathAliases",
		empty:       func(s Summary) bool { return len(s.ReturnParamPathAliases) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ReturnParamPathAliases = cloneSlice(src.ReturnParamPathAliases) },
		normalizeOwned: func(_ *axis.Registry, s *Summary) {
			s.ReturnParamPathAliases = returnParamPathAliasLane.Normalize(s.ReturnParamPathAliases)
		},
	},
	{
		fieldName:   "ParamSinkExposures",
		empty:       func(s Summary) bool { return len(s.ParamSinkExposures) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamSinkExposures = cloneSlice(src.ParamSinkExposures) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.ParamSinkExposures = normalizeParamSinkExposures(reg, s.ParamSinkExposures)
		},
	},
	{
		fieldName:   "CapturedPathObligations",
		empty:       func(s Summary) bool { return len(s.CapturedPathObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.CapturedPathObligations = cloneSlice(src.CapturedPathObligations) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.CapturedPathObligations = normalizeCapturedPathObligations(reg, s.CapturedPathObligations)
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
	},
	{
		fieldName:   "NormalReturnFacts",
		empty:       func(s Summary) bool { return s.NormalReturnFacts.Empty() },
		assignClone: func(src Summary, dst *Summary) { dst.NormalReturnFacts = cloneNormalReturnFacts(src.NormalReturnFacts) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.NormalReturnFacts = normalizeOwnedNormalReturnFacts(reg, s.NormalReturnFacts)
		},
	},
	{
		fieldName:   "HeapTableObjects",
		empty:       func(s Summary) bool { return len(s.HeapTableObjects) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.HeapTableObjects = cloneHeapTableObjects(src.HeapTableObjects) },
		normalizeOwned: func(reg *axis.Registry, s *Summary) {
			s.HeapTableObjects = normalizeOwnedHeapTableObjects(reg, s.HeapTableObjects)
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
