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
	labelChannelLifecycleCall  = "channel lifecycle call"
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

func appendDistinctSourceLabel(labels []diagnostic.Label, span, primary diagnostic.Span, message string) []diagnostic.Label {
	if span.Valid() && !diagnosticSpanEqual(span, primary) {
		return append(labels, sourceLabel(span, message))
	}
	return labels
}

var sourceLabelPlacements = map[string]diagnostic.LabelPlacement{
	labelCallTarget:            diagnostic.LabelPlacementBelow,
	labelCallExpression:        diagnostic.LabelPlacementBelow,
	labelCallResult:            diagnostic.LabelPlacementBelow,
	labelArgumentValue:         diagnostic.LabelPlacementBelow,
	labelExtraArgument:         diagnostic.LabelPlacementBelow,
	labelReturnedValue:         diagnostic.LabelPlacementBelow,
	labelMethodCall:            diagnostic.LabelPlacementBelow,
	labelMemberRead:            diagnostic.LabelPlacementBelow,
	labelMemberCall:            diagnostic.LabelPlacementBelow,
	labelAssignedValue:         diagnostic.LabelPlacementBelow,
	labelObjectLiteral:         diagnostic.LabelPlacementBelow,
	labelPossiblyNilContainer:  diagnostic.LabelPlacementBelow,
	labelValueMayBeNil:         diagnostic.LabelPlacementBelow,
	labelValueAlwaysNil:        diagnostic.LabelPlacementBelow,
	labelUnusedLocal:           diagnostic.LabelPlacementBelow,
	labelDeadAssignment:        diagnostic.LabelPlacementBelow,
	labelConditionCheck:        diagnostic.LabelPlacementBelow,
	labelUnknownType:           diagnostic.LabelPlacementBelow,
	labelUnknownValue:          diagnostic.LabelPlacementBelow,
	labelChannelCaseTest:       diagnostic.LabelPlacementBelow,
	labelChannelLifecycleCall:  diagnostic.LabelPlacementBelow,
	labelUnionCaseTest:         diagnostic.LabelPlacementBelow,
	labelOptionalCaseCheck:     diagnostic.LabelPlacementBelow,
	labelResultFieldRead:       diagnostic.LabelPlacementBelow,
	labelDispatchLookup:        diagnostic.LabelPlacementBelow,
	labelRegistrationCall:      diagnostic.LabelPlacementBelow,
	labelDispatchCall:          diagnostic.LabelPlacementBelow,
	labelFrozenTableMutation:   diagnostic.LabelPlacementBelow,
	labelFrozenTableCall:       diagnostic.LabelPlacementBelow,
	labelLifecycleAcquire:      diagnostic.LabelPlacementBelow,
	labelLifecycleTransition:   diagnostic.LabelPlacementBelow,
	labelLifecycleEscape:       diagnostic.LabelPlacementBelow,
	labelSendPayload:           diagnostic.LabelPlacementBelow,
	labelAdviceClaim:           diagnostic.LabelPlacementBelow,
	labelAdviceGuard:           diagnostic.LabelPlacementBelow,
	labelAdviceLoopRead:        diagnostic.LabelPlacementBelow,
	labelAdviceTagWrite:        diagnostic.LabelPlacementBelow,
	labelAdvicePayloadWrite:    diagnostic.LabelPlacementBelow,
	labelAdviceDiscriminantUse: diagnostic.LabelPlacementBelow,

	labelCalleeDeclaration: diagnostic.LabelPlacementAbove,
	labelDeclaredType:      diagnostic.LabelPlacementAbove,
	labelDeclaredReturn:    diagnostic.LabelPlacementAbove,
	labelAssignmentTarget:  diagnostic.LabelPlacementAbove,
	labelOverwrite:         diagnostic.LabelPlacementAbove,
	labelExitBeforeRead:    diagnostic.LabelPlacementAbove,
	labelProvingGuard:      diagnostic.LabelPlacementAbove,
	labelDispatchTable:     diagnostic.LabelPlacementAbove,
	labelFreezeProof:       diagnostic.LabelPlacementAbove,
	labelSendSafetyProof:   diagnostic.LabelPlacementAbove,
	labelAdviceProvenValue: diagnostic.LabelPlacementAbove,
	labelAdviceLoopHead:    diagnostic.LabelPlacementAbove,
	labelAdviceTableBirth:  diagnostic.LabelPlacementAbove,
}

func sourceLabelPlacement(message string) diagnostic.LabelPlacement {
	if placement, ok := sourceLabelPlacements[message]; ok {
		return placement
	}
	return diagnostic.LabelPlacementAuto
}
