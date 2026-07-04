package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// paramMemberCallObligationLane is the canonical may (union) membership lattice
// for parameter member-call obligations.
var paramMemberCallObligationLane = factset.Set[paramMemberCallObligationKey, ParamMemberCallObligation]{
	Key: paramMemberCallObligationSemanticKey,
	EqualFact: func(a, b ParamMemberCallObligation) bool {
		return paramMemberCallObligationSemanticKey(a) == paramMemberCallObligationSemanticKey(b)
	},
	Less: func(a, b ParamMemberCallObligation) bool {
		if a.ReceiverParam != b.ReceiverParam {
			return a.ReceiverParam < b.ReceiverParam
		}
		if a.ReceiverPath != b.ReceiverPath {
			return a.ReceiverPath < b.ReceiverPath
		}
		if a.ArgParam != b.ArgParam {
			return a.ArgParam < b.ArgParam
		}
		if a.Member != b.Member {
			return memberSegmentLess(a.Member, b.Member)
		}
		return a.MemberParamIndex < b.MemberParamIndex
	},
	Valid: func(o ParamMemberCallObligation) bool {
		if o.ReceiverParam < 0 || o.ArgParam < 0 || o.MemberParamIndex < 0 || !memberSegmentValid(o.Member) {
			return false
		}
		return o.ReceiverPath == "" || o.ReceiverPath.Valid()
	},
	Prefer: func(kept, incoming ParamMemberCallObligation) bool {
		return paramMemberCallObligationLabelScore(incoming) > paramMemberCallObligationLabelScore(kept)
	},
}

type paramMemberCallObligationKey struct {
	ReceiverParam    int
	ReceiverPath     pathaddr.SuffixKey
	Member           segment.Segment
	ArgParam         int
	MemberParamIndex int
}

func paramMemberCallObligationSemanticKey(o ParamMemberCallObligation) paramMemberCallObligationKey {
	return paramMemberCallObligationKey{
		ReceiverParam:    o.ReceiverParam,
		ReceiverPath:     o.ReceiverPath,
		Member:           o.Member,
		ArgParam:         o.ArgParam,
		MemberParamIndex: o.MemberParamIndex,
	}
}

func paramMemberCallObligationLabelScore(o ParamMemberCallObligation) int {
	score := 0
	if o.SubjectLabel != "" {
		score++
	}
	if o.ProviderLabel != "" {
		score++
	}
	return score
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
		return memberSegmentLess(a.Member, b.Member)
	},
	Valid: func(s ParamMemberReturnSlot) bool {
		return s.ReceiverParam >= 0 && s.ReturnIndex >= 0 && s.MemberResultIndex >= 0 && memberSegmentValid(s.Member)
	},
	Intersect: true,
}

func memberSegmentValid(seg segment.Segment) bool {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name != ""
	case segment.SegmentIndexInt:
		return true
	default:
		return false
	}
}

func memberSegmentLess(a, b segment.Segment) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Index < b.Index
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
	// An empty Member is the return-root alias (return o): the whole return slot,
	// not a member below it, aliases the parameter. The heap-member rehydration
	// consumer no-ops on it; the call-boundary exposure lane widens the argument
	// toward the full declared return type.
	Valid: func(a ReturnParamPathAlias) bool {
		return a.ReturnIndex >= 0 && (a.Member == "" || a.Member.Valid()) && a.Source.Valid()
	},
	Intersect: true,
}
