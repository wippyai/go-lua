package analysis

import (
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

// AnalyzeDiagnosticPhase is the coarse permanent production phase reached by
// one flagged Plan solve. It is scalar-only: detailed runtime rows remain in
// Engine and are allocated only when engine diagnostics are enabled.
type AnalyzeDiagnosticPhase uint8

const (
	AnalyzeDiagnosticPhaseNone AnalyzeDiagnosticPhase = iota
	AnalyzeDiagnosticPhaseSetup
	AnalyzeDiagnosticPhaseTopology
	AnalyzeDiagnosticPhaseItemIssuance
	AnalyzeDiagnosticPhaseActivation
	AnalyzeDiagnosticPhaseSourceSeal
	AnalyzeDiagnosticPhaseQueryPlan
	AnalyzeDiagnosticPhaseAssemble
	AnalyzeDiagnosticPhaseSolve
	AnalyzeDiagnosticPhaseObservation
	AnalyzeDiagnosticPhaseDetach
	AnalyzeDiagnosticPhaseComplete
)

func (phase AnalyzeDiagnosticPhase) String() string {
	switch phase {
	case AnalyzeDiagnosticPhaseSetup:
		return "setup"
	case AnalyzeDiagnosticPhaseTopology:
		return "topology"
	case AnalyzeDiagnosticPhaseItemIssuance:
		return "item-issuance"
	case AnalyzeDiagnosticPhaseActivation:
		return "activation"
	case AnalyzeDiagnosticPhaseSourceSeal:
		return "source-seal"
	case AnalyzeDiagnosticPhaseQueryPlan:
		return "query-plan"
	case AnalyzeDiagnosticPhaseAssemble:
		return "assemble"
	case AnalyzeDiagnosticPhaseSolve:
		return "solve"
	case AnalyzeDiagnosticPhaseObservation:
		return "observation"
	case AnalyzeDiagnosticPhaseDetach:
		return "detach"
	case AnalyzeDiagnosticPhaseComplete:
		return "complete"
	default:
		return "none"
	}
}

// AnalyzeDiagnosticReason classifies the terminal result at Phase. It is
// deliberately closed so fixture regressions do not rely on fragile strings.
type AnalyzeDiagnosticReason uint8

const (
	AnalyzeDiagnosticReasonNone AnalyzeDiagnosticReason = iota
	AnalyzeDiagnosticReasonInvalidPlan
	AnalyzeDiagnosticReasonInvalidOptions
	AnalyzeDiagnosticReasonConstruction
	AnalyzeDiagnosticReasonEngineIncomplete
	AnalyzeDiagnosticReasonEngineCanceled
	AnalyzeDiagnosticReasonEnginePanicked
	AnalyzeDiagnosticReasonWorkCutoff
	AnalyzeDiagnosticReasonObservation
	AnalyzeDiagnosticReasonDetach
)

func (reason AnalyzeDiagnosticReason) String() string {
	switch reason {
	case AnalyzeDiagnosticReasonInvalidPlan:
		return "invalid-plan"
	case AnalyzeDiagnosticReasonInvalidOptions:
		return "invalid-options"
	case AnalyzeDiagnosticReasonConstruction:
		return "construction"
	case AnalyzeDiagnosticReasonEngineIncomplete:
		return "engine-incomplete"
	case AnalyzeDiagnosticReasonEngineCanceled:
		return "engine-canceled"
	case AnalyzeDiagnosticReasonEnginePanicked:
		return "engine-panicked"
	case AnalyzeDiagnosticReasonWorkCutoff:
		return "work-cutoff"
	case AnalyzeDiagnosticReasonObservation:
		return "observation"
	case AnalyzeDiagnosticReasonDetach:
		return "detach"
	default:
		return "none"
	}
}

// AnalyzeDiagnosticReceiptStage identifies the receipt-native boundary last
// reached before a solve became incomplete. It is permanent scalar evidence;
// no runtime rows or callbacks are retained.
type AnalyzeDiagnosticReceiptStage uint8

const (
	AnalyzeDiagnosticReceiptStageNone AnalyzeDiagnosticReceiptStage = iota
	AnalyzeDiagnosticReceiptStageBinding
	AnalyzeDiagnosticReceiptStageMount
	AnalyzeDiagnosticReceiptStageLowering
	AnalyzeDiagnosticReceiptStageCommit
	AnalyzeDiagnosticReceiptStageRuntime
	AnalyzeDiagnosticReceiptStageSolve
	AnalyzeDiagnosticReceiptStageArtifactRules
	AnalyzeDiagnosticReceiptStageSourceSeal
	AnalyzeDiagnosticReceiptStageQueryRows
	AnalyzeDiagnosticReceiptStageArtifactRows
	AnalyzeDiagnosticReceiptStageQueryPlan
	AnalyzeDiagnosticReceiptStageBootstrapRules
)

// AnalyzeDiagnosticItemIssuanceFailure identifies the exact immutable row
// family that could not be issued before shared topology construction. It is
// intentionally scalar and permanent: fixture failures can name their owner
// without adding trace rows or changing compilation behavior.
type AnalyzeDiagnosticItemIssuanceFailure uint8

const (
	AnalyzeDiagnosticItemIssuanceFailureNone AnalyzeDiagnosticItemIssuanceFailure = iota
	AnalyzeDiagnosticItemIssuanceFailureProgramSchema
	AnalyzeDiagnosticItemIssuanceFailureArtifacts
	AnalyzeDiagnosticItemIssuanceFailureValueCoordinates
	AnalyzeDiagnosticItemIssuanceFailureDiagnosticObservations
	AnalyzeDiagnosticItemIssuanceFailureResultReceipt
)

func (failure AnalyzeDiagnosticItemIssuanceFailure) String() string {
	switch failure {
	case AnalyzeDiagnosticItemIssuanceFailureProgramSchema:
		return "program-schema"
	case AnalyzeDiagnosticItemIssuanceFailureArtifacts:
		return "artifacts"
	case AnalyzeDiagnosticItemIssuanceFailureValueCoordinates:
		return "value-coordinates"
	case AnalyzeDiagnosticItemIssuanceFailureDiagnosticObservations:
		return "diagnostic-observations"
	case AnalyzeDiagnosticItemIssuanceFailureResultReceipt:
		return "result-receipt"
	default:
		return "none"
	}
}

// AnalyzeDiagnosticRule is the closed analyzer-owned classification of the
// Engine first-failure Rule key. It is the sealed rule table's own
// classification: the inventory, the ordinals, and the names all come from
// that one table, so a rule added there is classified without a second list.
// Unknown includes empty, foreign, and generic engine lifecycle failures
// without a bound analyzer rule.
type AnalyzeDiagnosticRule = grammar.DiagnosticRule

const AnalyzeDiagnosticRuleUnknown = grammar.DiagnosticRuleUnknown

// AnalyzeDiagnostics is the detached analysis-level envelope for one Plan
// solve. Engine contains optional bounded runtime evidence; Phase and Reason
// remain available without event allocation.
type AnalyzeDiagnostics struct {
	Phase                     AnalyzeDiagnosticPhase
	Reason                    AnalyzeDiagnosticReason
	ItemIssuance              AnalyzeDiagnosticItemIssuanceFailure
	Rule                      AnalyzeDiagnosticRule
	ReceiptStage              AnalyzeDiagnosticReceiptStage
	Binding                   ProgramBindingFailure
	ValueSeal                 valuedomain.SealFailure
	AllocationCatalog         allocationcatalog.SealFailure
	ReceiptArtifactRow        engine.ReceiptArtifactRowFailure
	ReceiptOrdinal            uint32
	ReceiptSourceSeal         engine.ReceiptSourceSealFailure
	ReceiptRuleSourceSeal     engine.RuleSourceSealFailure
	ReceiptRuleFinalizer      engine.RuleFinalizerFailure
	ReceiptCommit             engine.ReceiptCommitFailurePhase
	ReceiptCommitPrecondition engine.ReceiptCommitPrecondition
	ReceiptCommitSemanticRows engine.ReceiptCommitSemanticRowsFailure
	ReceiptTopology           engine.ReceiptTopologyFailure
	ReceiptSchedule           engine.ReceiptScheduleFailure
	ReceiptScheduleOrdinal    uint32
	ReceiptCommitPublish      engine.ReceiptCommitPublishFailure
	ReceiptLowering           engine.ReceiptAssemblyFailure
	ObservationAttach         engine.ReceiptObservationAttachFailure
	Engine                    engine.SolveDiagnostics
}

func (diagnostics *AnalyzeDiagnostics) enter(phase AnalyzeDiagnosticPhase) {
	if diagnostics != nil {
		diagnostics.Phase = phase
	}
}

func (diagnostics *AnalyzeDiagnostics) fail(reason AnalyzeDiagnosticReason) {
	if diagnostics != nil && diagnostics.Reason == AnalyzeDiagnosticReasonNone {
		diagnostics.Reason = reason
	}
}

func (diagnostics *AnalyzeDiagnostics) failCurrentPhase() {
	if diagnostics == nil || diagnostics.Reason != AnalyzeDiagnosticReasonNone {
		return
	}
	switch diagnostics.Phase {
	case AnalyzeDiagnosticPhaseObservation:
		diagnostics.fail(AnalyzeDiagnosticReasonObservation)
	case AnalyzeDiagnosticPhaseSolve:
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
	default:
		diagnostics.fail(AnalyzeDiagnosticReasonConstruction)
	}
}
