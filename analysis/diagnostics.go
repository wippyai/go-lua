package analysis

import (
	"github.com/wippyai/go-lua/analysis/domain/composite"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
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

// ProgramBindingFailure is the closed Link-local binding boundary. It names
// only the owner transaction that rejected; no Schema slot, callback,
// coordinate, Program proof, or mutable binding state escapes diagnostics.
//
// The per-rule ordinals are derived from the sealed rule table rather than
// restated: a rule failure occupies programBindingFailureRuleBase plus that
// rule's diagnostic ordinal.
type ProgramBindingFailure uint8

const (
	ProgramBindingFailureNone ProgramBindingFailure = iota
	ProgramBindingFailureInput
	ProgramBindingFailureSemantics
	ProgramBindingFailureTypes
	ProgramBindingFailureStatic
	// ProgramBindingFailureAxisAuthority is the one axis-authority verdict: an
	// axis's own mount did not seal its Link authority. Which axis is the
	// declaration table's to name, so the rejecting axis travels beside this
	// verdict in AnalyzeDiagnostics.Axis rather than as one verdict member per
	// coordinate space.
	ProgramBindingFailureAxisAuthority
	// ProgramBindingFailureRuntimeContexts is the runtime allocation context
	// owner's own rejection: it joins the already-sealed factor pair and is a
	// stage of the binding transaction rather than an axis authority.
	ProgramBindingFailureRuntimeContexts
	ProgramBindingFailureHeapIndex
	ProgramBindingFailureTarget
	ProgramBindingFailureTargetCatalog
	ProgramBindingFailureTable
	ProgramBindingFailureReceipt
	ProgramBindingFailureBinding
	ProgramBindingFailurePrincipal
	ProgramBindingFailureAllocationCatalog
	ProgramBindingFailureQueryCatalog
	ProgramBindingFailureSeal
	ProgramBindingFailureAllocations
	ProgramBindingFailureValueQueryReceipt
	ProgramBindingFailureEffectQueryReceipt
	// programBindingFailureRuleBase is the first ordinal of the derived
	// per-rule tail. Nothing is declared past it.
	programBindingFailureRuleBase
)

var programBindingFailureNames = [...]string{
	"none", "input", "semantics", "types", "static",
	"axis-authority", "runtime-contexts", "heap-index",
	"target", "target-catalog", "table", "receipt", "binding", "principal",
	"allocation-catalog", "query-catalog", "seal", "allocations",
	"value-query-receipt", "effect-query-receipt",
}

func (failure ProgramBindingFailure) String() string {
	if failure >= programBindingFailureRuleBase {
		return "rule/" + composite.DiagnosticRule(failure-programBindingFailureRuleBase).String()
	}
	if int(failure) >= len(programBindingFailureNames) {
		return "invalid"
	}
	return programBindingFailureNames[failure]
}

func programBindingFailureForRule(rule composite.DiagnosticRule) ProgramBindingFailure {
	return programBindingFailureRuleBase + ProgramBindingFailure(rule)
}

// programBindingFailure projects the grammar's closed verdict into the
// analyzer's own boundary. A per-rule phase keeps the exact rule identity.
func programBindingFailure(failure composite.BindFailure) ProgramBindingFailure {
	switch failure.Stage {
	case composite.BindStageInput:
		return ProgramBindingFailureInput
	case composite.BindStageTable:
		return ProgramBindingFailureTable
	case composite.BindStageCompilation:
		return ProgramBindingFailureReceipt
	case composite.BindStageBinding:
		return ProgramBindingFailureBinding
	case composite.BindStagePrincipal:
		return ProgramBindingFailurePrincipal
	case composite.BindStageAllocationCatalog:
		return ProgramBindingFailureAllocationCatalog
	case composite.BindStageRule:
		return programBindingFailureForRule(failure.Rule)
	case composite.BindStageQueries:
		return ProgramBindingFailureQueryCatalog
	case composite.BindStageSeal:
		return ProgramBindingFailureSeal
	case composite.BindStageAllocations:
		return ProgramBindingFailureAllocations
	case composite.BindStageValueQueryReceipt:
		return ProgramBindingFailureValueQueryReceipt
	case composite.BindStageEffectQueryReceipt:
		return ProgramBindingFailureEffectQueryReceipt
	case composite.BindStageRuntimeContexts:
		return ProgramBindingFailureRuntimeContexts
	default:
		return ProgramBindingFailureNone
	}
}

// programMountFailure projects the mount phase's closed verdict into the
// analyzer's own boundary. An axis phase rejected one axis's own authority, and
// which axis it was is the declaration table's identity rather than a
// coordinate space this boundary re-enumerates, so the verdict is one and the
// axis travels with it.
//
// A post-mount derivation is owned by no axis, so it keeps its own verdict:
// which derivation refused is the phase's whole evidence, and each is spelled
// here as the boundary member that derivation has always published.
func programMountFailure(failure composite.MountFailure) ProgramBindingFailure {
	if !failure.Available() {
		return ProgramBindingFailureNone
	}
	switch failure.Stage {
	case composite.MountStageTopology:
		return ProgramBindingFailureHeapIndex
	case composite.MountStageActivation:
		return ProgramBindingFailureTargetCatalog
	case composite.MountStageAxis:
		if failure.Axis == composite.DiagnosticAxisUnknown {
			return ProgramBindingFailureInput
		}
		return ProgramBindingFailureAxisAuthority
	default:
		return ProgramBindingFailureInput
	}
}

// AnalyzeDiagnosticRule is the closed analyzer-owned classification of the
// Engine first-failure Rule key. It is the sealed rule table's own
// classification: the inventory, the ordinals, and the names all come from
// that one table, so a rule added there is classified without a second list.
// Unknown includes empty, foreign, and generic engine lifecycle failures
// without a bound analyzer rule.
type AnalyzeDiagnosticRule = composite.DiagnosticRule

const AnalyzeDiagnosticRuleUnknown = composite.DiagnosticRuleUnknown

// AnalyzeDiagnosticAxis is the closed analyzer-owned classification of one
// axis. It is the sealed axis table's own classification: the inventory, the
// slots, and the names all come from that one table, so an axis added there is
// classified without a second list. Unknown includes empty, foreign, and
// generic lifecycle failures without a bound analyzer axis.
type AnalyzeDiagnosticAxis = composite.DiagnosticAxis

const AnalyzeDiagnosticAxisUnknown = composite.DiagnosticAxisUnknown

// AnalyzeDiagnostics is the detached analysis-level envelope for one Plan
// solve. Engine contains optional bounded runtime evidence; Phase and Reason
// remain available without event allocation.
type AnalyzeDiagnostics struct {
	Phase        AnalyzeDiagnosticPhase
	Reason       AnalyzeDiagnosticReason
	ItemIssuance AnalyzeDiagnosticItemIssuanceFailure
	Rule         AnalyzeDiagnosticRule
	// Axis names the coordinate space a per-axis verdict is about. It is the
	// identity half of ProgramBindingFailureAxisAuthority.
	Axis              AnalyzeDiagnosticAxis
	ReceiptStage      AnalyzeDiagnosticReceiptStage
	Binding           ProgramBindingFailure
	ValueSeal         valuedomain.SealFailure
	AllocationCatalog allocationcatalog.SealFailure
	// ReceiptSeal, ReceiptLowering, ReceiptCommit, and ObservationAttach each
	// carry one engine boundary as its lifecycle family, universal
	// disposition, and opaque site identity. The engine's internal boundary
	// tables stay inside the engine; two boundaries are distinguished here by
	// their site digests. ObservationAttach spans the whole ordered runtime
	// attach path: a compile-family value names the attach phase that
	// rejected, an observation-family value the branch observation itself.
	ReceiptSeal            engine.SolveFailure
	ReceiptOrdinal         uint32
	ReceiptLowering        engine.SolveFailure
	ReceiptCommit          engine.SolveFailure
	ReceiptScheduleOrdinal uint32
	ObservationAttach      engine.SolveFailure
	Engine                 engine.SolveDiagnostics
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
