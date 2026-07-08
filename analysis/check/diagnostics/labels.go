package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

const (
	labelCallTarget            = "call target"
	labelCalleeDeclaration     = "callee declaration"
	labelCallExpression        = "call expression"
	labelCallResult            = "call result"
	labelArgumentValue         = "argument value"
	labelExtraArgument         = "extra argument"
	labelReturnedValue         = "returned value"
	labelMemberRead            = "member read"
	labelMemberCall            = "member call"
	labelMethodCall            = "method call"
	labelAssignedValue         = "assigned value"
	labelDeclaredType          = "declared type"
	labelDeclaredReturn        = "declared return type"
	labelAssignmentTarget      = "assignment target"
	labelObjectLiteral         = "object literal"
	labelPossiblyNilContainer  = "possibly nil container"
	labelValueMayBeNil         = "value may be nil"
	labelValueAlwaysNil        = "value is always nil"
	labelUnusedLocal           = "unused local"
	labelDeadAssignment        = "dead assignment"
	labelOverwrite             = "overwriting assignment"
	labelExitBeforeRead        = "exit before read"
	labelConditionCheck        = "current check"
	labelProvingGuard          = "prior guard"
	labelUnknownType           = "unknown type"
	labelUnknownValue          = "unknown value"
	labelChannelCaseTest       = "channel case check"
	labelUnionCaseTest         = "union case check"
	labelOptionalCaseCheck     = "optional case check"
	labelResultFieldRead       = "case-specific field read"
	labelDispatchLookup        = "dispatch lookup"
	labelDispatchTable         = "dispatch table"
	labelRegistrationCall      = "registration call"
	labelDispatchCall          = "dispatch call"
	labelFrozenTableMutation   = "mutation of frozen table"
	labelFrozenTableCall       = "mutating call on frozen table"
	labelFreezeProof           = "freeze proof"
	labelLifecycleAcquire      = "resource acquired"
	labelLifecycleTransition   = "lifecycle transition"
	labelLifecycleEscape       = "ownership escaped"
	labelSendPayload           = "send payload"
	labelSendSafetyProof       = "send-safety proof"
	labelAdviceClaim           = "claim site"
	labelAdviceProvenValue     = "proven value"
	labelAdviceGuard           = "constant guard"
	labelAdviceLoopRead        = "loop read"
	labelAdviceLoopHead        = "loop head"
	labelAdviceTagWrite        = "tag write"
	labelAdviceTableBirth      = "table birth"
	labelAdvicePayloadWrite    = "payload write"
	labelAdviceDiscriminantUse = "discriminant use"
)

func sourceLabel(span diagnostic.Span, message string) diagnostic.Label {
	return diagnostic.Label{Span: span, Message: message, Placement: sourceLabelPlacement(message)}
}

func sourceLabelPlacement(message string) diagnostic.LabelPlacement {
	switch message {
	case labelCallTarget,
		labelCallExpression,
		labelCallResult,
		labelArgumentValue,
		labelExtraArgument,
		labelReturnedValue,
		labelMethodCall,
		labelMemberRead,
		labelMemberCall,
		labelAssignedValue,
		labelObjectLiteral,
		labelPossiblyNilContainer,
		labelValueMayBeNil,
		labelValueAlwaysNil,
		labelUnusedLocal,
		labelDeadAssignment,
		labelConditionCheck,
		labelUnknownType,
		labelUnknownValue,
		labelChannelCaseTest,
		labelUnionCaseTest,
		labelOptionalCaseCheck,
		labelResultFieldRead,
		labelDispatchLookup,
		labelRegistrationCall,
		labelDispatchCall,
		labelFrozenTableMutation,
		labelFrozenTableCall,
		labelLifecycleAcquire,
		labelLifecycleTransition,
		labelLifecycleEscape,
		labelSendPayload,
		labelAdviceClaim,
		labelAdviceGuard,
		labelAdviceLoopRead,
		labelAdviceTagWrite,
		labelAdvicePayloadWrite,
		labelAdviceDiscriminantUse:
		return diagnostic.LabelPlacementBelow
	case labelCalleeDeclaration,
		labelDeclaredType,
		labelDeclaredReturn,
		labelAssignmentTarget,
		labelOverwrite,
		labelExitBeforeRead,
		labelProvingGuard,
		labelDispatchTable,
		labelFreezeProof,
		labelSendSafetyProof,
		labelAdviceProvenValue,
		labelAdviceLoopHead,
		labelAdviceTableBirth:
		return diagnostic.LabelPlacementAbove
	default:
		return diagnostic.LabelPlacementAuto
	}
}
