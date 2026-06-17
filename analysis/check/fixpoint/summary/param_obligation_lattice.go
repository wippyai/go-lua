package summary

import "github.com/wippyai/go-lua/analysis/domain/lattice/factset"

// paramMemberCallObligationLane is the canonical may (union) membership lattice
// for parameter member-call obligations.
var paramMemberCallObligationLane = factset.Set[ParamMemberCallObligation, ParamMemberCallObligation]{
	Key:       func(o ParamMemberCallObligation) ParamMemberCallObligation { return o },
	EqualFact: func(a, b ParamMemberCallObligation) bool { return a == b },
	Less: func(a, b ParamMemberCallObligation) bool {
		if a.ReceiverParam != b.ReceiverParam {
			return a.ReceiverParam < b.ReceiverParam
		}
		if a.ArgParam != b.ArgParam {
			return a.ArgParam < b.ArgParam
		}
		if a.Member != b.Member {
			return a.Member < b.Member
		}
		return a.MemberParamIndex < b.MemberParamIndex
	},
	Valid: func(o ParamMemberCallObligation) bool {
		return o.ReceiverParam >= 0 && o.ArgParam >= 0 && o.MemberParamIndex >= 0 && o.Member != ""
	},
}

// paramMemberReturnSlotLane is the canonical must (intersection) membership
// lattice for parameter member return slots.
var paramMemberReturnSlotLane = factset.Set[ParamMemberReturnSlot, ParamMemberReturnSlot]{
	Key:       func(s ParamMemberReturnSlot) ParamMemberReturnSlot { return s },
	EqualFact: func(a, b ParamMemberReturnSlot) bool { return a == b },
	Less: func(a, b ParamMemberReturnSlot) bool {
		if a.ReceiverParam != b.ReceiverParam {
			return a.ReceiverParam < b.ReceiverParam
		}
		if a.ReturnIndex != b.ReturnIndex {
			return a.ReturnIndex < b.ReturnIndex
		}
		if a.MemberResultIndex != b.MemberResultIndex {
			return a.MemberResultIndex < b.MemberResultIndex
		}
		return a.Member < b.Member
	},
	Valid: func(s ParamMemberReturnSlot) bool {
		return s.ReceiverParam >= 0 && s.ReturnIndex >= 0 && s.MemberResultIndex >= 0 && s.Member != ""
	},
	Intersect: true,
}

// returnParamPathAliasLane is the canonical must (intersection) membership
// lattice for return-parameter path aliases.
var returnParamPathAliasLane = factset.Set[ReturnParamPathAlias, ReturnParamPathAlias]{
	Key:       func(a ReturnParamPathAlias) ReturnParamPathAlias { return a },
	EqualFact: func(a, b ReturnParamPathAlias) bool { return a == b },
	Less: func(a, b ReturnParamPathAlias) bool {
		if a.ReturnIndex != b.ReturnIndex {
			return a.ReturnIndex < b.ReturnIndex
		}
		if a.Member != b.Member {
			return a.Member < b.Member
		}
		return a.Source < b.Source
	},
	Valid: func(a ReturnParamPathAlias) bool {
		return a.ReturnIndex >= 0 && a.Member != "" && a.Source != ""
	},
	Intersect: true,
}
