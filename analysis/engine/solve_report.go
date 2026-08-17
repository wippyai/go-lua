package engine

import "github.com/wippyai/go-lua/analysis/identity"

// SolveFailureReason identifies the first engine lifecycle boundary that made
// a solve incomplete.  It is deliberately a closed enum: callers receive no
// implementation error text, mutable runtime object, or second diagnostic
// authority.
type SolveFailureReason uint8

const (
	SolveFailureReasonNone SolveFailureReason = iota
	SolveFailureReasonEpoch
	// Activation relation merge failed or produced no new delta.
	SolveFailureReasonActivationMerge
	// The accepted activation revision could not compile.
	SolveFailureReasonActivationCompile
	// The solver activation revision counter is exhausted.
	SolveFailureReasonActivationRevisionOverflow
	// Evicting the prior retained publication failed.
	SolveFailureReasonActivationRetainedClose
	SolveFailureReasonExecution
	SolveFailureReasonQuery
	SolveFailureReasonPublication
)

// SolveFailurePhase identifies the engine lifecycle phase that made a solve
// incomplete. It covers both cold/runtime compilation and bound RuleMember
// execution. It carries no typed/domain diagnostic; the zero value is used
// when no more specific phase was reached.
type SolveFailurePhase uint8

const (
	SolveFailurePhaseNone SolveFailurePhase = iota
	// The sealed cold input or accepted activation graph was invalid.
	SolveFailurePhaseCompileValidation
	// The accepted graph could not produce an operand revision.
	SolveFailurePhaseCompileOperandRevision
	// Factor binding or runtime composition preparation failed.
	SolveFailurePhaseCompileComposition
	// A graph member could not bind to its sealed rule schema.
	SolveFailurePhaseCompileMemberBinding
	// A graph query could not bind to its sealed query schema.
	SolveFailurePhaseCompileQueryBinding
	// The bound rows could not be assembled into the runtime.
	SolveFailurePhaseCompileRuntimeAssembly
	SolveFailurePhasePreflight
	SolveFailurePhaseTransfer
	SolveFailurePhaseCheckpoint
	SolveFailurePhaseDerivation
	SolveFailurePhaseAdmission
	SolveFailurePhasePublication
	// Activation-specific preflight boundaries retain the exact failed
	// invariant without exposing the compiled callback or Product rows.
	SolveFailurePhaseActivationInstance
	SolveFailurePhaseActivationContribution
	SolveFailurePhaseActivationReads
	SolveFailurePhaseActivationEpoch
	SolveFailurePhaseActivationProduct
	// RefreshPoint could not validate its Point/runtime ownership boundary.
	SolveFailurePhaseRefreshValidation
	// RefreshPoint could not evaluate or authenticate a producer candidate.
	SolveFailurePhaseRefreshCandidate
	// A producer candidate failed the local monotonic/order boundary.
	SolveFailurePhaseRefreshCandidateOrder
	// RefreshPoint could not replace demand or commit a candidate generation.
	SolveFailurePhaseRefreshDemandCommit
	// The acyclic structural-input ascent certificate was unavailable.
	SolveFailurePhaseRefreshAcyclicStructuralInputs
	// An acyclic Point could not acquire its canonical Init/base RHS.
	SolveFailurePhaseRefreshAcyclicPointBase
	// An acyclic Point could not validate the foldPoint boundary.
	SolveFailurePhaseRefreshAcyclicFoldPoint
	// foldPointTerms could not begin its canonical RHS transaction.
	SolveFailurePhaseRefreshAcyclicFoldBegin
	// foldPointTerms could not transport an environment input.
	SolveFailurePhaseRefreshAcyclicFoldEnvironment
	// A Factor fold edge had an invalid descriptor, bounds, or input plan.
	SolveFailurePhaseRefreshAcyclicFoldFactorValidation
	// A Factor fold edge could not project its source PointState.
	SolveFailurePhaseRefreshAcyclicFoldFactorProjection
	// A Factor point transport failed its owner/scope preflight.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportPreflight
	// A coordinate-identical Factor transport failed its pre-support filter.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePreSupport
	// A coordinate-identical Factor transport failed support reindexing.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateReindexSupport
	// A coordinate-identical Factor transport failed its post-support filter.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePostSupport
	// A coordinate-identical Factor transport failed coverage transport.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateCoverage
	// A coordinate-identical Factor transport could not admit its output.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateAdmission
	// A general Factor transport failed its pre-state filter.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPreFilter
	// A general Factor transport failed State-reindex support preparation.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexSupport
	// A general Factor transport failed its typed all-slot reindex transaction.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexTypedSlots
	// A general Factor transport failed State-reindex commit.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexCommit
	// A general Factor transport failed its post-state filter.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPostFilter
	// A general Factor transport failed coverage transport.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralCoverage
	// A general Factor transport could not admit its output.
	SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralAdmission
	// A Factor fold edge could not admit its transported PointState to the fold.
	SolveFailurePhaseRefreshAcyclicFoldFactorAdmission
	// foldPointTerms could not add a producer contribution.
	SolveFailurePhaseRefreshAcyclicFoldProducer
	// foldPointTerms could not finish its canonical RHS transaction.
	SolveFailurePhaseRefreshAcyclicFoldFinish
	// An acyclic Point could not publish its folded RHS.
	SolveFailurePhaseRefreshAcyclicPublication
	// A recurrent Point could not validate or refresh its Region interface.
	SolveFailurePhaseRefreshRegionInterface
	// A recurrent Point could not construct its canonical Region RHS.
	SolveFailurePhaseRefreshRegionRHS
	// A recurrent Point failed its exact/current Region order boundary.
	SolveFailurePhaseRefreshRegionOrder
	// A recurrent Point could not merge exact/current Region state.
	SolveFailurePhaseRefreshRegionMerge
	// A recurrent Point could not publish its merged Region state.
	SolveFailurePhaseRefreshRegionPublication
	// The executor's WTO event traversal reported an invalid boundary.
	SolveFailurePhaseRunVisitInvalid
	// The executor retained queued work but visited no executable Point.
	SolveFailurePhaseRunVisitNoProgress
	// The demanded postfix discharge reported an invalid boundary.
	SolveFailurePhaseRunPostfixInvalid
	// Postfix remained unproved without scheduling further Point work.
	SolveFailurePhaseRunPostfixStalled
	// The ascent-to-narrow transition reported an invalid Region boundary.
	SolveFailurePhaseRunNarrowInvalid
	// The ascent-to-narrow transition made no Region progress.
	SolveFailurePhaseRunNarrowNoProgress
	// A changed producer candidate had identical read/environment generations.
	SolveFailurePhaseRefreshCandidateOrderStableInputs
	// A nonascending producer candidate had no live recurrent descent context.
	SolveFailurePhaseRefreshCandidateOrderRegion
	// A recurrent narrow candidate was incomparable with its prior value.
	SolveFailurePhaseRefreshCandidateOrderDescent
	// Optional read-only observation failed before entering the Factor read.
	SolveFailurePhaseObservationPreflight
	// Optional read-only observation could not open its Factor work generation.
	SolveFailurePhaseObservationWork
	// Optional read-only observation retained no live Factor unit.
	SolveFailurePhaseObservationUnit
	// Optional read-only observation retained no valid support region.
	SolveFailurePhaseObservationSupport
	// Optional read-only observation could not resolve the settled Factor root.
	SolveFailurePhaseObservationRoot
	// Optional read-only observation could not traverse the settled Factor state.
	SolveFailurePhaseObservationCarrier
	// Optional read-only observation could not decode a settled Factor row.
	SolveFailurePhaseObservationDecode
	// Optional read-only observation could not fold a decoded Factor row.
	SolveFailurePhaseObservationProjection
	// Optional read-only observation violated its declared row shape.
	SolveFailurePhaseObservationShape
	// Optional read-only observation could not freeze at the terminal checkpoint.
	SolveFailurePhaseObservationFreeze
)

// String returns a stable compact diagnostic spelling for a closed failure
// phase. Unknown values are intentionally not normalized into a known phase.
func (phase SolveFailurePhase) String() string {
	switch phase {
	case SolveFailurePhaseNone:
		return "none"
	case SolveFailurePhaseCompileValidation:
		return "compile-validation"
	case SolveFailurePhaseCompileOperandRevision:
		return "compile-operand-revision"
	case SolveFailurePhaseCompileComposition:
		return "compile-composition"
	case SolveFailurePhaseCompileMemberBinding:
		return "compile-member-binding"
	case SolveFailurePhaseCompileQueryBinding:
		return "compile-query-binding"
	case SolveFailurePhaseCompileRuntimeAssembly:
		return "compile-runtime-assembly"
	case SolveFailurePhasePreflight:
		return "preflight"
	case SolveFailurePhaseTransfer:
		return "transfer"
	case SolveFailurePhaseCheckpoint:
		return "checkpoint"
	case SolveFailurePhaseDerivation:
		return "derivation"
	case SolveFailurePhaseAdmission:
		return "admission"
	case SolveFailurePhasePublication:
		return "publication"
	case SolveFailurePhaseActivationInstance:
		return "activation-instance"
	case SolveFailurePhaseActivationContribution:
		return "activation-contribution"
	case SolveFailurePhaseActivationReads:
		return "activation-reads"
	case SolveFailurePhaseActivationEpoch:
		return "activation-epoch"
	case SolveFailurePhaseActivationProduct:
		return "activation-product"
	case SolveFailurePhaseRefreshValidation:
		return "refresh-validation"
	case SolveFailurePhaseRefreshCandidate:
		return "refresh-candidate"
	case SolveFailurePhaseRefreshCandidateOrder:
		return "refresh-candidate-order"
	case SolveFailurePhaseRefreshDemandCommit:
		return "refresh-demand-commit"
	case SolveFailurePhaseRefreshAcyclicStructuralInputs:
		return "refresh-acyclic-structural-inputs"
	case SolveFailurePhaseRefreshAcyclicPointBase:
		return "refresh-acyclic-point-base"
	case SolveFailurePhaseRefreshAcyclicFoldPoint:
		return "refresh-acyclic-fold-point"
	case SolveFailurePhaseRefreshAcyclicFoldBegin:
		return "refresh-acyclic-fold-begin"
	case SolveFailurePhaseRefreshAcyclicFoldEnvironment:
		return "refresh-acyclic-fold-environment"
	case SolveFailurePhaseRefreshAcyclicFoldFactorValidation:
		return "refresh-acyclic-fold-factor-validation"
	case SolveFailurePhaseRefreshAcyclicFoldFactorProjection:
		return "refresh-acyclic-fold-factor-projection"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportPreflight:
		return "refresh-acyclic-fold-factor-transport-preflight"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePreSupport:
		return "refresh-acyclic-fold-factor-transport-coordinate-pre-support"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateReindexSupport:
		return "refresh-acyclic-fold-factor-transport-coordinate-reindex-support"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePostSupport:
		return "refresh-acyclic-fold-factor-transport-coordinate-post-support"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateCoverage:
		return "refresh-acyclic-fold-factor-transport-coordinate-coverage"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateAdmission:
		return "refresh-acyclic-fold-factor-transport-coordinate-admission"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPreFilter:
		return "refresh-acyclic-fold-factor-transport-general-pre-filter"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexSupport:
		return "refresh-acyclic-fold-factor-transport-general-reindex-support"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexTypedSlots:
		return "refresh-acyclic-fold-factor-transport-general-reindex-typed-slots"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexCommit:
		return "refresh-acyclic-fold-factor-transport-general-reindex-commit"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPostFilter:
		return "refresh-acyclic-fold-factor-transport-general-post-filter"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralCoverage:
		return "refresh-acyclic-fold-factor-transport-general-coverage"
	case SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralAdmission:
		return "refresh-acyclic-fold-factor-transport-general-admission"
	case SolveFailurePhaseRefreshAcyclicFoldFactorAdmission:
		return "refresh-acyclic-fold-factor-admission"
	case SolveFailurePhaseRefreshAcyclicFoldProducer:
		return "refresh-acyclic-fold-producer"
	case SolveFailurePhaseRefreshAcyclicFoldFinish:
		return "refresh-acyclic-fold-finish"
	case SolveFailurePhaseRefreshAcyclicPublication:
		return "refresh-acyclic-publication"
	case SolveFailurePhaseRefreshRegionInterface:
		return "refresh-region-interface"
	case SolveFailurePhaseRefreshRegionRHS:
		return "refresh-region-rhs"
	case SolveFailurePhaseRefreshRegionOrder:
		return "refresh-region-order"
	case SolveFailurePhaseRefreshRegionMerge:
		return "refresh-region-merge"
	case SolveFailurePhaseRefreshRegionPublication:
		return "refresh-region-publication"
	case SolveFailurePhaseRunVisitInvalid:
		return "run-visit-invalid"
	case SolveFailurePhaseRunVisitNoProgress:
		return "run-visit-no-progress"
	case SolveFailurePhaseRunPostfixInvalid:
		return "run-postfix-invalid"
	case SolveFailurePhaseRunPostfixStalled:
		return "run-postfix-stalled"
	case SolveFailurePhaseRunNarrowInvalid:
		return "run-narrow-invalid"
	case SolveFailurePhaseRunNarrowNoProgress:
		return "run-narrow-no-progress"
	case SolveFailurePhaseRefreshCandidateOrderStableInputs:
		return "refresh-candidate-order-stable-inputs"
	case SolveFailurePhaseRefreshCandidateOrderRegion:
		return "refresh-candidate-order-region"
	case SolveFailurePhaseRefreshCandidateOrderDescent:
		return "refresh-candidate-order-descent"
	case SolveFailurePhaseObservationPreflight:
		return "observation-preflight"
	case SolveFailurePhaseObservationWork:
		return "observation-work"
	case SolveFailurePhaseObservationUnit:
		return "observation-unit"
	case SolveFailurePhaseObservationSupport:
		return "observation-support"
	case SolveFailurePhaseObservationRoot:
		return "observation-root"
	case SolveFailurePhaseObservationCarrier:
		return "observation-carrier"
	case SolveFailurePhaseObservationDecode:
		return "observation-decode"
	case SolveFailurePhaseObservationProjection:
		return "observation-projection"
	case SolveFailurePhaseObservationShape:
		return "observation-shape"
	case SolveFailurePhaseObservationFreeze:
		return "observation-freeze"
	default:
		return "unknown"
	}
}

// SolveReport is a detached first-failure certificate for one incomplete
// solve.  Every coordinate is an opaque identity.SemanticKey copied out of the sealed
// engine authority.  The zero value means that no incomplete solve was
// reported; all fields are private so the report cannot be forged or retain a
// Solver, State, callback, domain value, or mutable slice.
type SolveReport struct {
	reason SolveFailureReason
	phase  SolveFailurePhase
	point  identity.SemanticKey
	group  identity.SemanticKey
	member identity.SemanticKey
	rule   identity.SemanticKey
}

// Available reports whether this report is a certificate for SolveIncomplete.
func (report SolveReport) Available() bool { return report.reason != SolveFailureReasonNone }

// Reason returns the first lifecycle boundary recorded by the solver.
func (report SolveReport) Reason() SolveFailureReason { return report.reason }

// Phase returns the engine lifecycle phase that failed. It is None when the
// report identifies a boundary without a more specific phase.
func (report SolveReport) Phase() SolveFailurePhase { return report.phase }

// Point returns the failed Point semantic identity when the boundary had one.
func (report SolveReport) Point() identity.SemanticKey { return report.point }

// Group returns the failed Group semantic identity when the boundary had one.
func (report SolveReport) Group() identity.SemanticKey { return report.group }

// Member returns the failed RuleMember semantic identity when member execution
// reached one.
func (report SolveReport) Member() identity.SemanticKey { return report.member }

// Rule returns the failed Rule semantic identity when member execution reached
// one.
func (report SolveReport) Rule() identity.SemanticKey { return report.rule }

func (report *SolveReport) record(reason SolveFailureReason, phase SolveFailurePhase, point, group, member, rule identity.SemanticKey) {
	if report == nil || report.Available() || reason == SolveFailureReasonNone {
		return
	}
	report.reason = reason
	report.phase = phase
	report.point = point
	report.group = group
	report.member = member
	report.rule = rule
}

func reportFailureQuery(report *SolveReport, reason SolveFailureReason, point identity.SemanticKey) {
	if report != nil {
		report.record(reason, SolveFailurePhaseNone, point, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
	}
}
