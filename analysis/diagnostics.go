package analysis

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/composite"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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

// AnalyzeDiagnosticReceiptStage identifies the boundary last reached before a
// solve became incomplete. It is permanent scalar evidence; no runtime rows or
// callbacks are retained. Ordinals are display-only: nothing persists, hashes,
// or transports a member, so the set grows by appending.
//
// Two groups share the vocabulary. The content stages name what was being
// constructed - the rows, the seal, the plan - and keep their meaning across
// the construction path they are reached from. The construction stages name
// the boundaries of the program constructor itself: the pass that takes a
// committed topology plus the rows a bind produced and mints the sealed
// program and its Solver. They are the analyzer's half of
// engine.ProgramConstructionStage.
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
	// AnalyzeDiagnosticReceiptStageAdmission is the constructor's input fence:
	// the construction handle, the committed topology, and the sealed directory
	// that topology published. The mounted artifact schedule gate runs at the
	// topology commit, so its verdict reaches the constructor here.
	AnalyzeDiagnosticReceiptStageAdmission
	// AnalyzeDiagnosticReceiptStageTopologySeal is the topology commit itself:
	// the mounted artifact schedule gate, the graph, and the directory the
	// admission fence re-checks.
	AnalyzeDiagnosticReceiptStageTopologySeal
	// AnalyzeDiagnosticReceiptStageQueryAddress is the published query address
	// table: one canonical key per directory ordinal over the graph's queries.
	AnalyzeDiagnosticReceiptStageQueryAddress
	// AnalyzeDiagnosticReceiptStageObservationAddress is the published
	// observation address table: one row per admitted attach identity.
	AnalyzeDiagnosticReceiptStageObservationAddress
	// AnalyzeDiagnosticReceiptStageFactorBind is the bound Factor table: the
	// dense owner index and each bound Factor's runtime slot.
	AnalyzeDiagnosticReceiptStageFactorBind
	// AnalyzeDiagnosticReceiptStageMemberBind is the member bind: the per-Group
	// fold of cold draft answers and the hot rows minted from those drafts.
	AnalyzeDiagnosticReceiptStageMemberBind
	// AnalyzeDiagnosticReceiptStageProgramSeal is the seal of the row-model
	// program and the runtime assembled from it.
	AnalyzeDiagnosticReceiptStageProgramSeal
	// AnalyzeDiagnosticReceiptStageSolverMint is the Solver mint: the initial
	// relation and the one store issuance its addresses are named in.
	AnalyzeDiagnosticReceiptStageSolverMint
)

var analyzeDiagnosticReceiptStageNames = [...]string{
	"none", "binding", "mount", "lowering", "commit", "runtime", "solve",
	"artifact-rules", "source-seal", "query-rows", "artifact-rows",
	"query-plan", "bootstrap-rules",
	"admission", "topology-seal", "query-address", "observation-address",
	"factor-bind", "member-bind", "program-seal", "solver-mint",
}

func (stage AnalyzeDiagnosticReceiptStage) String() string {
	if int(stage) >= len(analyzeDiagnosticReceiptStageNames) {
		return "invalid"
	}
	return analyzeDiagnosticReceiptStageNames[stage]
}

// analyzeDiagnosticConstructionStage projects the engine's program
// construction boundary onto the analyzer's own stage vocabulary. A boundary
// raised anywhere else in the compile family names no construction stage and
// leaves the caller's stage as it stood.
func analyzeDiagnosticConstructionStage(failure engine.SolveFailure) (AnalyzeDiagnosticReceiptStage, bool) {
	stage, named := engine.ProgramConstructionStageOf(failure)
	if !named {
		return AnalyzeDiagnosticReceiptStageNone, false
	}
	switch stage {
	case engine.ProgramConstructionStageAdmission:
		return AnalyzeDiagnosticReceiptStageAdmission, true
	case engine.ProgramConstructionStageTopologySeal:
		return AnalyzeDiagnosticReceiptStageTopologySeal, true
	case engine.ProgramConstructionStageQueryAddress:
		return AnalyzeDiagnosticReceiptStageQueryAddress, true
	case engine.ProgramConstructionStageObservationAddress:
		return AnalyzeDiagnosticReceiptStageObservationAddress, true
	case engine.ProgramConstructionStageFactorBind:
		return AnalyzeDiagnosticReceiptStageFactorBind, true
	case engine.ProgramConstructionStageMemberBind:
		return AnalyzeDiagnosticReceiptStageMemberBind, true
	case engine.ProgramConstructionStageProgramSeal:
		return AnalyzeDiagnosticReceiptStageProgramSeal, true
	case engine.ProgramConstructionStageSolverMint:
		return AnalyzeDiagnosticReceiptStageSolverMint, true
	default:
		return AnalyzeDiagnosticReceiptStageNone, false
	}
}

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
	AnalyzeDiagnosticItemIssuanceFailureResultGeometry
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
	case AnalyzeDiagnosticItemIssuanceFailureResultGeometry:
		return "result-geometry"
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
	ProgramBindingFailureQueryReceipt
	// programBindingFailureRuleBase is the first ordinal of the derived
	// per-rule tail. Nothing is declared past it.
	programBindingFailureRuleBase
)

var programBindingFailureNames = [...]string{
	"none", "input", "types", "static",
	"axis-authority", "runtime-contexts", "heap-index",
	"target", "target-catalog", "table", "receipt", "binding", "principal",
	"allocation-catalog", "query-catalog", "seal", "allocations",
	"query-receipt",
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
	case composite.BindStageQueryReceipt:
		return ProgramBindingFailureQueryReceipt
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

// enterConstruction localizes one program construction refusal. The engine
// names the boundary in the failure it returns, so the analyzer keeps a single
// stage field rather than a second coordinate travelling beside it. A failure
// from any other authority leaves the stage the caller already recorded.
func (diagnostics *AnalyzeDiagnostics) enterConstruction(failure engine.SolveFailure) {
	if diagnostics == nil {
		return
	}
	if stage, named := analyzeDiagnosticConstructionStage(failure); named {
		diagnostics.ReceiptStage = stage
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
