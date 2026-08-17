package artifact

import "fmt"

// CompileStage is the closed cold-compiler phase vocabulary.
type CompileStage uint8

const (
	CompileStageInvalid CompileStage = iota
	CompileStageAuthority
	CompileStageValues
	CompileStageBodyOutcomes
	CompileStageLocalWTO
	CompileStageRoutes
	CompileStageCanonicalize
	CompileStageSeal
	CompileStageOccurrences
)

func (stage CompileStage) valid() bool {
	return stage >= CompileStageAuthority && stage <= CompileStageOccurrences
}

func (stage CompileStage) String() string {
	switch stage {
	case CompileStageAuthority:
		return "authority"
	case CompileStageValues:
		return "values"
	case CompileStageBodyOutcomes:
		return "body-outcomes"
	case CompileStageLocalWTO:
		return "local-wto"
	case CompileStageRoutes:
		return "routes"
	case CompileStageCanonicalize:
		return "canonicalize"
	case CompileStageSeal:
		return "seal"
	case CompileStageOccurrences:
		return "occurrences"
	default:
		return "invalid"
	}
}

// CompileRowKind names the exact immutable denominator being consumed.
type CompileRowKind uint8

const (
	CompileRowInvalid CompileRowKind = iota
	CompileRowAuthority
	CompileRowRegion
	CompileRowWTOEvent
	CompileRowPoint
	CompileRowRoute
	CompileRowEnvironment
	CompileRowValues
	CompileRowBody
	CompileRowOutcome
	CompileRowReturnValue
	CompileRowOccurrence
)

func (kind CompileRowKind) valid() bool {
	return kind >= CompileRowAuthority && kind <= CompileRowOccurrence
}

func (kind CompileRowKind) String() string {
	switch kind {
	case CompileRowAuthority:
		return "authority"
	case CompileRowRegion:
		return "region"
	case CompileRowWTOEvent:
		return "wto-event"
	case CompileRowPoint:
		return "point"
	case CompileRowRoute:
		return "route"
	case CompileRowEnvironment:
		return "environment"
	case CompileRowValues:
		return "values"
	case CompileRowBody:
		return "body"
	case CompileRowOutcome:
		return "outcome"
	case CompileRowReturnValue:
		return "return-value"
	case CompileRowOccurrence:
		return "occurrence"
	default:
		return "invalid"
	}
}

// CompileReason is a closed invariant-failure vocabulary. It contains no
// caller text and therefore cannot become a generic diagnostic side channel.
type CompileReason uint16

const (
	CompileReasonInvalid CompileReason = iota
	CompileReasonProgramUnavailable
	CompileReasonGrammarUnavailable
	CompileReasonCompileKeyUnavailable
	CompileReasonRegionUnavailable
	CompileReasonRegionDuplicate
	CompileReasonRegionHeaderUnavailable
	CompileReasonRegionEmpty
	CompileReasonRegionMemberUnavailable
	CompileReasonRegionMemberDuplicate
	CompileReasonRegionHeaderMismatch
	CompileReasonEventUnavailable
	CompileReasonEventRegionUnavailable
	CompileReasonEventRegionUnknown
	CompileReasonEventRegionRepeated
	CompileReasonEventRootParent
	CompileReasonEventParentMismatch
	CompileReasonEventPointUnavailable
	CompileReasonEventPointRepeated
	CompileReasonEventPointOrder
	CompileReasonEventExitUnavailable
	CompileReasonEventExitMismatch
	CompileReasonEventKindUnknown
	CompileReasonEventUnbalanced
	CompileReasonRegionIncomplete
	CompileReasonPointUnscheduled
	CompileReasonRouteUnavailable
	CompileReasonRouteForeign
	CompileReasonRouteEndpoints
	CompileReasonRouteIdentity
	CompileReasonRouteArm
	CompileReasonRouteGuard
	CompileReasonRouteMu
	CompileReasonRouteRecurrence
	CompileReasonRouteReset
	CompileReasonRouteResetMember
	CompileReasonRouteResetOrder
	CompileReasonRouteMuResetMismatch
	CompileReasonEnvironmentUnavailable
	CompileReasonEnvironmentDuplicate
	CompileReasonArtifactIdentity
	CompileReasonPointUnavailable
	CompileReasonPointOrder
	CompileReasonEnvironmentEndpointUnknown
	CompileReasonRegionReference
	CompileReasonEventReference
	CompileReasonValuesUnavailable
	CompileReasonValuesForeign
	CompileReasonValuesBody
	CompileReasonValuesIdentity
	CompileReasonValuesMember
	CompileReasonValuesTail
	CompileReasonValuesDuplicate
	CompileReasonBodyUnavailable
	CompileReasonBodyForeign
	CompileReasonBodyIdentity
	CompileReasonBodyDuplicate
	CompileReasonBodyRange
	CompileReasonOutcomeUnavailable
	CompileReasonOutcomeAttachment
	CompileReasonOutcomeShape
	CompileReasonOutcomeForeign
	CompileReasonOutcomeIdentity
	CompileReasonOutcomeDuplicate
	CompileReasonOutcomeKind
	CompileReasonOutcomeTarget
	CompileReasonOutcomePropagation
	CompileReasonOutcomeReference
	CompileReasonOutcomeBody
	CompileReasonOutcomeRange
	CompileReasonOutcomeReturn
	CompileReasonReturnValueUnavailable
	CompileReasonReturnValueReference
	CompileReasonOccurrenceUnavailable
	CompileReasonOccurrenceValues
	CompileReasonOccurrenceAttachment
	CompileReasonOccurrenceValueSource
	CompileReasonOccurrenceValueSourceProof
	CompileReasonOccurrenceValueSourceOwner
	CompileReasonOccurrenceValueSourceBody
	CompileReasonOccurrenceValueSourceFinish
	CompileReasonOccurrenceValueSourcePoints
	CompileReasonOccurrenceValueSourceAppend
	CompileReasonOccurrenceStorage
	CompileReasonOccurrenceStorageRead
	CompileReasonOccurrenceStorageBind
	CompileReasonOccurrenceStorageAssignment
	CompileReasonOccurrenceIndex
	CompileReasonOccurrenceIndexCandidate
	CompileReasonOccurrenceIndexShape
	CompileReasonOccurrenceIndexAppend
	CompileReasonOccurrenceAllocation
	CompileReasonOccurrenceCall
)

func (reason CompileReason) valid() bool {
	return reason >= CompileReasonProgramUnavailable && reason <= CompileReasonOccurrenceCall
}

func (reason CompileReason) String() string {
	switch reason {
	case CompileReasonProgramUnavailable:
		return "program-unavailable"
	case CompileReasonGrammarUnavailable:
		return "grammar-unavailable"
	case CompileReasonCompileKeyUnavailable:
		return "compile-key-unavailable"
	case CompileReasonRegionUnavailable:
		return "region-unavailable"
	case CompileReasonRegionDuplicate:
		return "region-duplicate"
	case CompileReasonRegionHeaderUnavailable:
		return "region-header-unavailable"
	case CompileReasonRegionEmpty:
		return "region-empty"
	case CompileReasonRegionMemberUnavailable:
		return "region-member-unavailable"
	case CompileReasonRegionMemberDuplicate:
		return "region-member-duplicate"
	case CompileReasonRegionHeaderMismatch:
		return "region-header-mismatch"
	case CompileReasonEventUnavailable:
		return "event-unavailable"
	case CompileReasonEventRegionUnavailable:
		return "event-region-unavailable"
	case CompileReasonEventRegionUnknown:
		return "event-region-unknown"
	case CompileReasonEventRegionRepeated:
		return "event-region-repeated"
	case CompileReasonEventRootParent:
		return "event-root-has-parent"
	case CompileReasonEventParentMismatch:
		return "event-parent-mismatch"
	case CompileReasonEventPointUnavailable:
		return "event-point-unavailable"
	case CompileReasonEventPointRepeated:
		return "event-point-repeated"
	case CompileReasonEventPointOrder:
		return "event-point-order"
	case CompileReasonEventExitUnavailable:
		return "event-exit-unavailable"
	case CompileReasonEventExitMismatch:
		return "event-exit-mismatch"
	case CompileReasonEventKindUnknown:
		return "event-kind-unknown"
	case CompileReasonEventUnbalanced:
		return "event-unbalanced"
	case CompileReasonRegionIncomplete:
		return "region-incomplete"
	case CompileReasonPointUnscheduled:
		return "point-unscheduled"
	case CompileReasonRouteUnavailable:
		return "route-unavailable"
	case CompileReasonRouteForeign:
		return "route-foreign"
	case CompileReasonRouteEndpoints:
		return "route-endpoints"
	case CompileReasonRouteIdentity:
		return "route-identity"
	case CompileReasonRouteArm:
		return "route-arm"
	case CompileReasonRouteGuard:
		return "route-guard"
	case CompileReasonRouteMu:
		return "route-mu"
	case CompileReasonRouteRecurrence:
		return "route-recurrence"
	case CompileReasonRouteReset:
		return "route-reset"
	case CompileReasonRouteResetMember:
		return "route-reset-member"
	case CompileReasonRouteResetOrder:
		return "route-reset-order"
	case CompileReasonRouteMuResetMismatch:
		return "route-mu-reset-mismatch"
	case CompileReasonEnvironmentUnavailable:
		return "environment-unavailable"
	case CompileReasonEnvironmentDuplicate:
		return "environment-duplicate"
	case CompileReasonArtifactIdentity:
		return "artifact-identity"
	case CompileReasonPointUnavailable:
		return "point-unavailable"
	case CompileReasonPointOrder:
		return "point-order"
	case CompileReasonEnvironmentEndpointUnknown:
		return "environment-endpoint-unknown"
	case CompileReasonRegionReference:
		return "region-reference"
	case CompileReasonEventReference:
		return "event-reference"
	case CompileReasonValuesUnavailable:
		return "values-unavailable"
	case CompileReasonValuesForeign:
		return "values-foreign"
	case CompileReasonValuesBody:
		return "values-body"
	case CompileReasonValuesIdentity:
		return "values-identity"
	case CompileReasonValuesMember:
		return "values-member"
	case CompileReasonValuesTail:
		return "values-tail"
	case CompileReasonValuesDuplicate:
		return "values-duplicate"
	case CompileReasonBodyUnavailable:
		return "body-unavailable"
	case CompileReasonBodyForeign:
		return "body-foreign"
	case CompileReasonBodyIdentity:
		return "body-identity"
	case CompileReasonBodyDuplicate:
		return "body-duplicate"
	case CompileReasonBodyRange:
		return "body-range"
	case CompileReasonOutcomeUnavailable:
		return "outcome-unavailable"
	case CompileReasonOutcomeAttachment:
		return "outcome-attachment"
	case CompileReasonOutcomeShape:
		return "outcome-shape"
	case CompileReasonOutcomeForeign:
		return "outcome-foreign"
	case CompileReasonOutcomeIdentity:
		return "outcome-identity"
	case CompileReasonOutcomeDuplicate:
		return "outcome-duplicate"
	case CompileReasonOutcomeKind:
		return "outcome-kind"
	case CompileReasonOutcomeTarget:
		return "outcome-target"
	case CompileReasonOutcomePropagation:
		return "outcome-propagation"
	case CompileReasonOutcomeReference:
		return "outcome-reference"
	case CompileReasonOutcomeBody:
		return "outcome-body"
	case CompileReasonOutcomeRange:
		return "outcome-range"
	case CompileReasonOutcomeReturn:
		return "outcome-return"
	case CompileReasonReturnValueUnavailable:
		return "return-value-unavailable"
	case CompileReasonReturnValueReference:
		return "return-value-reference"
	case CompileReasonOccurrenceUnavailable:
		return "occurrence-unavailable"
	case CompileReasonOccurrenceValues:
		return "occurrence-values"
	case CompileReasonOccurrenceAttachment:
		return "occurrence-attachment"
	case CompileReasonOccurrenceValueSource:
		return "occurrence-value-source"
	case CompileReasonOccurrenceValueSourceProof:
		return "occurrence-value-source-proof"
	case CompileReasonOccurrenceValueSourceOwner:
		return "occurrence-value-source-owner"
	case CompileReasonOccurrenceValueSourceBody:
		return "occurrence-value-source-body"
	case CompileReasonOccurrenceValueSourceFinish:
		return "occurrence-value-source-finish"
	case CompileReasonOccurrenceValueSourcePoints:
		return "occurrence-value-source-points"
	case CompileReasonOccurrenceValueSourceAppend:
		return "occurrence-value-source-append"
	case CompileReasonOccurrenceStorage:
		return "occurrence-storage"
	case CompileReasonOccurrenceStorageRead:
		return "occurrence-storage-read"
	case CompileReasonOccurrenceStorageBind:
		return "occurrence-storage-bind"
	case CompileReasonOccurrenceStorageAssignment:
		return "occurrence-storage-assignment"
	case CompileReasonOccurrenceIndex:
		return "occurrence-index"
	case CompileReasonOccurrenceIndexCandidate:
		return "occurrence-index-candidate"
	case CompileReasonOccurrenceIndexShape:
		return "occurrence-index-shape"
	case CompileReasonOccurrenceIndexAppend:
		return "occurrence-index-append"
	case CompileReasonOccurrenceAllocation:
		return "occurrence-allocation"
	case CompileReasonOccurrenceCall:
		return "occurrence-call"
	default:
		return "invalid"
	}
}

// CompileFailure is an immutable compilation failure. A zero value means success.
// Row and Subrow are stable positions in the exact parent denominator only;
// no Program/Flow proof or builder escapes.
type CompileFailure struct {
	stage  CompileStage
	kind   CompileRowKind
	reason CompileReason
	row    int
	subrow int
}

func compileFailure(stage CompileStage, kind CompileRowKind, row, subrow int, reason CompileReason) CompileFailure {
	failure := CompileFailure{stage: stage, kind: kind, reason: reason, row: row, subrow: subrow}
	if !failure.Available() {
		return CompileFailure{}
	}
	return failure
}

func (failure CompileFailure) Available() bool {
	return failure.stage.valid() && failure.kind.valid() && failure.reason.valid() && failure.row >= -1 && failure.subrow >= -1
}

func (failure CompileFailure) Stage() CompileStage     { return failure.stage }
func (failure CompileFailure) RowKind() CompileRowKind { return failure.kind }
func (failure CompileFailure) Reason() CompileReason   { return failure.reason }
func (failure CompileFailure) Row() (int, bool) {
	return failure.row, failure.Available() && failure.row >= 0
}
func (failure CompileFailure) Subrow() (int, bool) {
	return failure.subrow, failure.Available() && failure.subrow >= 0
}

func (failure CompileFailure) Error() string {
	if !failure.Available() {
		return "program artifact compile succeeded"
	}
	return fmt.Sprintf("program artifact compile: stage=%s row-kind=%s row=%d subrow=%d reason=%s", failure.stage, failure.kind, failure.row, failure.subrow, failure.reason)
}
