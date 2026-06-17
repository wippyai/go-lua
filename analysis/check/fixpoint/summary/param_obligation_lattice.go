package summary

import "sort"

func normalizeParamMemberCallObligations(in []ParamMemberCallObligation) []ParamMemberCallObligation {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[ParamMemberCallObligation]struct{}, len(in))
	out := make([]ParamMemberCallObligation, 0, len(in))
	for _, obligation := range in {
		if obligation.ReceiverParam < 0 || obligation.ArgParam < 0 ||
			obligation.MemberParamIndex < 0 || obligation.Member == "" {
			continue
		}
		if _, ok := seen[obligation]; ok {
			continue
		}
		seen[obligation] = struct{}{}
		out = append(out, obligation)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceiverParam != out[j].ReceiverParam {
			return out[i].ReceiverParam < out[j].ReceiverParam
		}
		if out[i].ArgParam != out[j].ArgParam {
			return out[i].ArgParam < out[j].ArgParam
		}
		if out[i].Member != out[j].Member {
			return out[i].Member < out[j].Member
		}
		return out[i].MemberParamIndex < out[j].MemberParamIndex
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeParamMemberReturnSlots(in []ParamMemberReturnSlot) []ParamMemberReturnSlot {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[ParamMemberReturnSlot]struct{}, len(in))
	out := make([]ParamMemberReturnSlot, 0, len(in))
	for _, slot := range in {
		if slot.ReceiverParam < 0 || slot.ReturnIndex < 0 ||
			slot.MemberResultIndex < 0 || slot.Member == "" {
			continue
		}
		if _, ok := seen[slot]; ok {
			continue
		}
		seen[slot] = struct{}{}
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceiverParam != out[j].ReceiverParam {
			return out[i].ReceiverParam < out[j].ReceiverParam
		}
		if out[i].ReturnIndex != out[j].ReturnIndex {
			return out[i].ReturnIndex < out[j].ReturnIndex
		}
		if out[i].MemberResultIndex != out[j].MemberResultIndex {
			return out[i].MemberResultIndex < out[j].MemberResultIndex
		}
		return out[i].Member < out[j].Member
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func paramMemberCallObligationsEqual(a, b []ParamMemberCallObligation) bool {
	a = normalizeParamMemberCallObligations(a)
	b = normalizeParamMemberCallObligations(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func paramMemberCallObligationsLessOrEq(a, b []ParamMemberCallObligation) bool {
	a = normalizeParamMemberCallObligations(a)
	b = normalizeParamMemberCallObligations(b)
	if len(a) == 0 {
		return true
	}
	if len(b) == 0 {
		return false
	}
	have := make(map[ParamMemberCallObligation]struct{}, len(b))
	for _, obligation := range b {
		have[obligation] = struct{}{}
	}
	for _, obligation := range a {
		if _, ok := have[obligation]; !ok {
			return false
		}
	}
	return true
}

func joinParamMemberCallObligations(a, b []ParamMemberCallObligation) []ParamMemberCallObligation {
	if len(a) == 0 {
		return normalizeParamMemberCallObligations(b)
	}
	if len(b) == 0 {
		return normalizeParamMemberCallObligations(a)
	}
	out := make([]ParamMemberCallObligation, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return normalizeParamMemberCallObligations(out)
}

func paramMemberReturnSlotsEqual(a, b []ParamMemberReturnSlot) bool {
	a = normalizeParamMemberReturnSlots(a)
	b = normalizeParamMemberReturnSlots(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func paramMemberReturnSlotsLessOrEq(a, b []ParamMemberReturnSlot) bool {
	a = normalizeParamMemberReturnSlots(a)
	b = normalizeParamMemberReturnSlots(b)
	if len(b) == 0 {
		return true
	}
	if len(a) == 0 {
		return false
	}
	have := make(map[ParamMemberReturnSlot]struct{}, len(a))
	for _, slot := range a {
		have[slot] = struct{}{}
	}
	for _, slot := range b {
		if _, ok := have[slot]; !ok {
			return false
		}
	}
	return true
}

func joinParamMemberReturnSlots(a, b []ParamMemberReturnSlot) []ParamMemberReturnSlot {
	a = normalizeParamMemberReturnSlots(a)
	b = normalizeParamMemberReturnSlots(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	bSlots := make(map[ParamMemberReturnSlot]struct{}, len(b))
	for _, slot := range b {
		bSlots[slot] = struct{}{}
	}
	out := make([]ParamMemberReturnSlot, 0, min(len(a), len(b)))
	for _, slot := range a {
		if _, ok := bSlots[slot]; ok {
			out = append(out, slot)
		}
	}
	return normalizeParamMemberReturnSlots(out)
}
