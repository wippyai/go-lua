package summary

type summaryLane struct {
	fieldName string
	slot      bool
	empty     func(Summary) bool
}

var summaryLanes = []summaryLane{
	{
		fieldName: "Returns",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.Returns) == 0 },
	},
	{
		fieldName: "ParamObligations",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.ParamObligations) == 0 },
	},
	{
		fieldName: "ParamMemberCallObligations",
		empty:     func(s Summary) bool { return len(s.ParamMemberCallObligations) == 0 },
	},
	{
		fieldName: "ParamMemberReturnSlots",
		empty:     func(s Summary) bool { return len(s.ParamMemberReturnSlots) == 0 },
	},
	{
		fieldName: "ReturnParamPathAliases",
		empty:     func(s Summary) bool { return len(s.ReturnParamPathAliases) == 0 },
	},
	{
		fieldName: "ParamSinkExposures",
		empty:     func(s Summary) bool { return len(s.ParamSinkExposures) == 0 },
	},
	{
		fieldName: "CapturedPathObligations",
		empty:     func(s Summary) bool { return len(s.CapturedPathObligations) == 0 },
	},
	{
		fieldName: "NormalReturnParams",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.NormalReturnParams) == 0 },
	},
	{
		fieldName: "NormalReturnParamConditions",
		slot:      true,
		empty:     func(s Summary) bool { return len(s.NormalReturnParamConditions) == 0 },
	},
	{
		fieldName: "NormalReturnParamEqualities",
		empty:     func(s Summary) bool { return len(s.NormalReturnParamEqualities) == 0 },
	},
	{
		fieldName: "NormalReturnFacts",
		empty:     func(s Summary) bool { return s.NormalReturnFacts.Empty() },
	},
	{
		fieldName: "HeapTableObjects",
		empty:     func(s Summary) bool { return len(s.HeapTableObjects) == 0 },
	},
	{
		fieldName: "ReturnConditionParamRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionParamRefinements) == 0 },
	},
	{
		fieldName: "ReturnConditionSlotRefinements",
		empty:     func(s Summary) bool { return len(s.ReturnConditionSlotRefinements) == 0 },
	},
	{
		fieldName: "ReturnParamLiteralCases",
		empty:     func(s Summary) bool { return len(s.ReturnParamLiteralCases) == 0 },
	},
	{
		fieldName: "ReturnPresenceRelations",
		empty:     func(s Summary) bool { return len(s.ReturnPresenceRelations) == 0 },
	},
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
