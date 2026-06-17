// Package summary defines fixed-point function summaries for analysis checks.
package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// Summary is the fixed-point analysis summary payload for one function entry.
type Summary struct {
	Returns                         []product.Value
	ParamObligations                []product.Value
	ParamMemberCallObligations      []ParamMemberCallObligation
	ParamMemberReturnSlots          []ParamMemberReturnSlot
	ReturnParamPathAliases          []ReturnParamPathAlias
	NormalReturnParams              []product.Value
	NormalReturnParamConditions     []ParamCondition
	NormalReturnParamEqualities     []ParamEquality
	NormalReturnFacts               callboundary.NormalReturnFacts
	HeapTableObjects                map[identity.ID]heapidentity.TableObject
	ReturnConditionParamRefinements []ReturnConditionParamRefinement
	ReturnPresenceRelations         []ReturnPresenceRelation
}

// Clone returns an independent copy of s.
func (s Summary) Clone() Summary {
	if len(s.Returns) == 0 &&
		len(s.ParamObligations) == 0 &&
		len(s.ParamMemberCallObligations) == 0 &&
		len(s.ParamMemberReturnSlots) == 0 &&
		len(s.ReturnParamPathAliases) == 0 &&
		len(s.NormalReturnParams) == 0 &&
		len(s.NormalReturnParamConditions) == 0 &&
		len(s.NormalReturnParamEqualities) == 0 &&
		normalReturnFactsEmpty(s.NormalReturnFacts) &&
		len(s.HeapTableObjects) == 0 &&
		len(s.ReturnConditionParamRefinements) == 0 &&
		len(s.ReturnPresenceRelations) == 0 {
		return Summary{}
	}
	out := Summary{}
	if len(s.Returns) > 0 {
		out.Returns = make([]product.Value, len(s.Returns))
		copy(out.Returns, s.Returns)
	}
	if len(s.ParamObligations) > 0 {
		out.ParamObligations = make([]product.Value, len(s.ParamObligations))
		copy(out.ParamObligations, s.ParamObligations)
	}
	if len(s.ParamMemberCallObligations) > 0 {
		out.ParamMemberCallObligations = make([]ParamMemberCallObligation, len(s.ParamMemberCallObligations))
		copy(out.ParamMemberCallObligations, s.ParamMemberCallObligations)
	}
	if len(s.ParamMemberReturnSlots) > 0 {
		out.ParamMemberReturnSlots = make([]ParamMemberReturnSlot, len(s.ParamMemberReturnSlots))
		copy(out.ParamMemberReturnSlots, s.ParamMemberReturnSlots)
	}
	if len(s.ReturnParamPathAliases) > 0 {
		out.ReturnParamPathAliases = make([]ReturnParamPathAlias, len(s.ReturnParamPathAliases))
		copy(out.ReturnParamPathAliases, s.ReturnParamPathAliases)
	}
	if len(s.NormalReturnParams) > 0 {
		out.NormalReturnParams = make([]product.Value, len(s.NormalReturnParams))
		copy(out.NormalReturnParams, s.NormalReturnParams)
	}
	if len(s.NormalReturnParamConditions) > 0 {
		out.NormalReturnParamConditions = make([]ParamCondition, len(s.NormalReturnParamConditions))
		copy(out.NormalReturnParamConditions, s.NormalReturnParamConditions)
	}
	if len(s.NormalReturnParamEqualities) > 0 {
		out.NormalReturnParamEqualities = make([]ParamEquality, len(s.NormalReturnParamEqualities))
		copy(out.NormalReturnParamEqualities, s.NormalReturnParamEqualities)
	}
	out.NormalReturnFacts = cloneNormalReturnFacts(s.NormalReturnFacts)
	out.HeapTableObjects = cloneHeapTableObjects(s.HeapTableObjects)
	out.ReturnConditionParamRefinements = cloneReturnConditionParamRefinements(s.ReturnConditionParamRefinements)
	out.ReturnPresenceRelations = cloneReturnPresenceRelations(s.ReturnPresenceRelations)
	return out
}
