package summary

type summaryLane struct {
	fieldName   string
	slot        bool
	empty       func(Summary) bool
	assignClone func(src Summary, dst *Summary)
}

var summaryLanes = []summaryLane{
	{
		fieldName:   "Returns",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.Returns) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.Returns = cloneSlice(src.Returns) },
	},
	{
		fieldName:   "ParamObligations",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.ParamObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamObligations = cloneSlice(src.ParamObligations) },
	},
	{
		fieldName: "ParamMemberCallObligations",
		empty:     func(s Summary) bool { return len(s.ParamMemberCallObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ParamMemberCallObligations = cloneSlice(src.ParamMemberCallObligations)
		},
	},
	{
		fieldName:   "ParamMemberReturnSlots",
		empty:       func(s Summary) bool { return len(s.ParamMemberReturnSlots) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamMemberReturnSlots = cloneSlice(src.ParamMemberReturnSlots) },
	},
	{
		fieldName:   "ReturnParamPathAliases",
		empty:       func(s Summary) bool { return len(s.ReturnParamPathAliases) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ReturnParamPathAliases = cloneSlice(src.ReturnParamPathAliases) },
	},
	{
		fieldName:   "ParamSinkExposures",
		empty:       func(s Summary) bool { return len(s.ParamSinkExposures) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.ParamSinkExposures = cloneSlice(src.ParamSinkExposures) },
	},
	{
		fieldName:   "CapturedPathObligations",
		empty:       func(s Summary) bool { return len(s.CapturedPathObligations) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.CapturedPathObligations = cloneSlice(src.CapturedPathObligations) },
	},
	{
		fieldName:   "NormalReturnParams",
		slot:        true,
		empty:       func(s Summary) bool { return len(s.NormalReturnParams) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.NormalReturnParams = cloneSlice(src.NormalReturnParams) },
	},
	{
		fieldName: "NormalReturnParamConditions",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.NormalReturnParamConditions) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.NormalReturnParamConditions = cloneSlice(src.NormalReturnParamConditions)
		},
	},
	{
		fieldName: "NormalReturnParamEqualities",
		empty:     func(s Summary) bool { return len(s.NormalReturnParamEqualities) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.NormalReturnParamEqualities = cloneSlice(src.NormalReturnParamEqualities)
		},
	},
	{
		fieldName:   "NormalReturnFacts",
		empty:       func(s Summary) bool { return s.NormalReturnFacts.Empty() },
		assignClone: func(src Summary, dst *Summary) { dst.NormalReturnFacts = cloneNormalReturnFacts(src.NormalReturnFacts) },
	},
	{
		fieldName:   "HeapTableObjects",
		empty:       func(s Summary) bool { return len(s.HeapTableObjects) == 0 },
		assignClone: func(src Summary, dst *Summary) { dst.HeapTableObjects = cloneHeapTableObjects(src.HeapTableObjects) },
	},
	{
		fieldName: "ReturnConditionParamRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionParamRefinements) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnConditionParamRefinements = cloneReturnConditionParamRefinements(src.ReturnConditionParamRefinements)
		},
	},
	{
		fieldName: "ReturnConditionSlotRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionSlotRefinements) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnConditionSlotRefinements = cloneReturnConditionSlotRefinements(src.ReturnConditionSlotRefinements)
		},
	},
	{
		fieldName: "ReturnParamLiteralCases",
		empty:     func(s Summary) bool { return len(s.ReturnParamLiteralCases) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnParamLiteralCases = cloneReturnParamLiteralCases(src.ReturnParamLiteralCases)
		},
	},
	{
		fieldName: "ReturnPresenceRelations",
		empty:     func(s Summary) bool { return len(s.ReturnPresenceRelations) == 0 },
		assignClone: func(src Summary, dst *Summary) {
			dst.ReturnPresenceRelations = returnPresenceRelationLane.Clone(src.ReturnPresenceRelations)
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
