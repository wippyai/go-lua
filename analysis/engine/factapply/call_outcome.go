package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

type CallResult = callpayload.CallResult

type CallOutcomeProvider = callpayload.CallOutcomeProvider

type CallOutcome = callpayload.CallOutcome

type CallParamObligation = callpayload.CallParamObligation

type CallParamPathRefinement = callpayload.CallParamPathRefinement

type CallParamLengthFloor = callpayload.CallParamLengthFloor

type CallParamPathInvalidation = callpayload.CallParamPathInvalidation

type CallParamCondition = callpayload.CallParamCondition

type CallPathRelationKind = callpayload.CallPathRelationKind

const (
	CallPathRelationEqual = callpayload.CallPathRelationEqual
)

type CallParamPathRelation = callpayload.CallParamPathRelation

type CallReturnConditionRefinement = callpayload.CallReturnConditionRefinement

type CallReturnPresenceRelation = callpayload.CallReturnPresenceRelation
