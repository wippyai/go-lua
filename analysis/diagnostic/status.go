package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/composite"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
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

// AnalyzeDiagnosticAssembleStage identifies the analysis-owned boundary last
// reached before a solve became incomplete. It names what the analyzer was
// assembling. Engine construction refusals travel as Construction: family,
// disposition, and an opaque site.
type AnalyzeDiagnosticAssembleStage uint8

const (
	AnalyzeDiagnosticAssembleStageNone AnalyzeDiagnosticAssembleStage = iota
	AnalyzeDiagnosticAssembleStageBinding
	AnalyzeDiagnosticAssembleStageMount
	AnalyzeDiagnosticAssembleStageLowering
	AnalyzeDiagnosticAssembleStageCommit
	AnalyzeDiagnosticAssembleStageRuntime
	AnalyzeDiagnosticAssembleStageSolve
	AnalyzeDiagnosticAssembleStageArtifactRules
	AnalyzeDiagnosticAssembleStageSourceSeal
	AnalyzeDiagnosticAssembleStageQueryRows
	AnalyzeDiagnosticAssembleStageArtifactRows
	AnalyzeDiagnosticAssembleStageQueryPlan
	AnalyzeDiagnosticAssembleStageBootstrapRules
)

var analyzeDiagnosticAssembleStageNames = [...]string{
	"none", "binding", "mount", "lowering", "commit", "runtime", "solve",
	"artifact-rules", "source-seal", "query-rows", "artifact-rows",
	"query-plan", "bootstrap-rules",
}

func (stage AnalyzeDiagnosticAssembleStage) String() string {
	if int(stage) >= len(analyzeDiagnosticAssembleStageNames) {
		return "invalid"
	}
	return analyzeDiagnosticAssembleStageNames[stage]
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
	ProgramBindingFailureHeapIndex
	ProgramBindingFailureTarget
	ProgramBindingFailureTable
	ProgramBindingFailureCompilation
	ProgramBindingFailureBinding
	ProgramBindingFailurePrincipal
	ProgramBindingFailureAllocationCatalog
	ProgramBindingFailureQueryCatalog
	ProgramBindingFailureSeal
	ProgramBindingFailureAllocations
	// programBindingFailureRuleBase is the first ordinal of the derived
	// per-rule tail. Nothing is declared past it.
	programBindingFailureRuleBase
)

var programBindingFailureNames = [...]string{
	"none", "input", "types", "static",
	"axis-authority", "heap-index",
	"target", "table", "compilation", "binding", "principal",
	"allocation-catalog", "query-catalog", "seal", "allocations",
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

func ProgramBindingFailureForRule(rule composite.DiagnosticRule) ProgramBindingFailure {
	return programBindingFailureRuleBase + ProgramBindingFailure(rule)
}

// ProgramBindingFailureFromBind projects the grammar's closed verdict into the
// analyzer's own boundary. A per-rule phase keeps the exact rule identity.
func ProgramBindingFailureFromBind(failure composite.BindFailure) ProgramBindingFailure {
	switch failure.Stage {
	case composite.BindStageInput:
		return ProgramBindingFailureInput
	case composite.BindStageTable:
		return ProgramBindingFailureTable
	case composite.BindStageCompilation:
		return ProgramBindingFailureCompilation
	case composite.BindStageBinding:
		return ProgramBindingFailureBinding
	case composite.BindStagePrincipal:
		return ProgramBindingFailurePrincipal
	case composite.BindStageAllocationCatalog:
		return ProgramBindingFailureAllocationCatalog
	case composite.BindStageRule:
		return ProgramBindingFailureForRule(failure.Rule)
	case composite.BindStageQueries:
		return ProgramBindingFailureQueryCatalog
	case composite.BindStageSeal:
		return ProgramBindingFailureSeal
	case composite.BindStageAllocations:
		return ProgramBindingFailureAllocations
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
// Post-mount derivations are owned by no axis, so each keeps its own verdict:
// which derivation refused is the phase's whole evidence, and each is spelled
// here as the boundary member that derivation publishes.
func ProgramBindingFailureFromMount(failure composite.MountFailure) ProgramBindingFailure {
	if !failure.Available() {
		return ProgramBindingFailureNone
	}
	switch failure.Stage {
	case composite.MountStageTopology:
		return ProgramBindingFailureHeapIndex
	case composite.MountStageFormal:
		return ProgramBindingFailureTarget
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

// AnalyzeDiagnosticRuleBindStage is the closed analyzer-owned classification of
// the binder pass that rejected one rule. It is the rule binder's own stage
// vocabulary: the rule table names which rule, this names which of that rule's
// passes.
type AnalyzeDiagnosticRuleBindStage = composite.RuleBindStage

const AnalyzeDiagnosticRuleBindStageNone = composite.RuleBindStageNone

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
	Axis          AnalyzeDiagnosticAxis
	AssembleStage AnalyzeDiagnosticAssembleStage
	Binding       ProgramBindingFailure
	// BindingRuleStage names the binder pass a per-rule verdict is about. It is
	// the pass half of a ProgramBindingFailure rule ordinal, which names only
	// which rule refused.
	BindingRuleStage AnalyzeDiagnosticRuleBindStage
	ValueSeal        valuedomain.SealFailure
	// PackSeal recovers a rejected pack-axis mount at pack's own evidence
	// type. A mount rejection from another axis leaves it absent: which axis
	// rejected is composite.MountFailure.Axis, and only that axis's own type
	// recovers its reason.
	PackSeal          packowner.MountRejection
	AllocationCatalog allocationcatalog.SealFailure
	// AssembleSeal, AssembleLowering, AssembleCommit, and ObservationAttach
	// each carry one engine boundary as its lifecycle family, universal
	// disposition, and opaque site identity. The engine's internal boundary
	// tables stay inside the engine; two boundaries are distinguished here by
	// their site digests. ObservationAttach spans the whole ordered runtime
	// attach path: a compile-family value names the attach phase that
	// rejected, an observation-family value the branch observation itself.
	//
	// AssembleSeal also carries the declaration-seal boundary of a refused
	// ProgramAdmissionLink, ProgramAdmissionMounted, or ProgramAdmissionQuery
	// stage, with AssembleOrdinal beside it naming the admission row that
	// stage rejected.
	AssembleSeal     engine.SolveFailure
	AssembleOrdinal  uint32
	AssembleLowering engine.SolveFailure
	AssembleCommit   engine.SolveFailure
	// AssembleScheduleOrdinal is the declared composition-schedule row a
	// refused construction stopped on; it is zero for a construction refused
	// at any other step. AssembleConstructionRow is that same refused
	// construction's declared row for every step, schedule included, so a
	// non-schedule construction step still publishes the row it stopped on.
	AssembleScheduleOrdinal uint32
	AssembleConstructionRow uint32
	ObservationAttach       engine.SolveFailure
	// Construction carries one program-constructor refusal as family,
	// disposition, and opaque site. The constructor's internal stage names
	// stay inside the engine.
	Construction engine.SolveFailure
	Engine       engine.SolveDiagnostics
}

func (diagnostics *AnalyzeDiagnostics) Enter(phase AnalyzeDiagnosticPhase) {
	if diagnostics != nil {
		diagnostics.Phase = phase
	}
}

// EnterConstruction records one program construction refusal. A
// foreign-family failure leaves Construction empty and AssembleStage as the
// caller already recorded.
func (diagnostics *AnalyzeDiagnostics) EnterConstruction(failure engine.SolveFailure) {
	if diagnostics == nil {
		return
	}
	if _, named := engine.ProgramSealStageOf(failure); named {
		diagnostics.Construction = failure
	}
}

// EnterProgramAssemble projects the engine's canonical program-construction
// refusal into the analysis envelope. Root callers provide the already-bound
// diagnostic rule because the engine intentionally keeps rule vocabularies
// opaque. A zero refusal is a root preflight failure and leaves the caller's
// stage untouched.
func (diagnostics *AnalyzeDiagnostics) EnterProgramAssemble(refusal engine.ProgramAssembleRefusal, rule AnalyzeDiagnosticRule) {
	if diagnostics == nil {
		return
	}
	if refusal.Lowered() {
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageLowering
		diagnostics.AssembleLowering = refusal.LoweringFailure()
		return
	}
	switch refusal.Stage() {
	case engine.ProgramAdmissionLink:
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageBootstrapRules
		diagnostics.AssembleSeal = refusal.Seal()
		diagnostics.AssembleOrdinal, _ = refusal.AdmissionRow()
	case engine.ProgramAdmissionMounted:
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageArtifactRules
		diagnostics.AssembleSeal = refusal.Seal()
		diagnostics.AssembleOrdinal, _ = refusal.AdmissionRow()
	case engine.ProgramAdmissionQuery:
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageQueryRows
		diagnostics.AssembleSeal = refusal.Seal()
		if ordinal, artifactRows := refusal.ArtifactRowOrdinal(); artifactRows {
			diagnostics.AssembleOrdinal = ordinal
		} else {
			diagnostics.AssembleOrdinal, _ = refusal.AdmissionRow()
		}
	case engine.ProgramAdmissionSeal:
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageSourceSeal
		diagnostics.AssembleSeal = refusal.Seal()
		if ordinal, artifactRows := refusal.ArtifactRowOrdinal(); artifactRows {
			diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageArtifactRows
			diagnostics.AssembleOrdinal = ordinal
		} else {
			diagnostics.Rule = rule
		}
	default:
		commit := refusal.Commit()
		if !commit.Available() && refusal.ScheduleRow() == 0 {
			return
		}
		diagnostics.AssembleStage = AnalyzeDiagnosticAssembleStageCommit
		diagnostics.AssembleCommit = commit
		diagnostics.AssembleScheduleOrdinal = refusal.ScheduleRow()
		diagnostics.AssembleConstructionRow, _ = refusal.ConstructionRow()
	}
}

func (diagnostics *AnalyzeDiagnostics) Fail(reason AnalyzeDiagnosticReason) {
	if diagnostics != nil && diagnostics.Reason == AnalyzeDiagnosticReasonNone {
		diagnostics.Reason = reason
	}
}

func (diagnostics *AnalyzeDiagnostics) FailCurrentPhase() {
	if diagnostics == nil || diagnostics.Reason != AnalyzeDiagnosticReasonNone {
		return
	}
	switch diagnostics.Phase {
	case AnalyzeDiagnosticPhaseObservation:
		diagnostics.Fail(AnalyzeDiagnosticReasonObservation)
	case AnalyzeDiagnosticPhaseSolve:
		diagnostics.Fail(AnalyzeDiagnosticReasonEngineIncomplete)
	default:
		diagnostics.Fail(AnalyzeDiagnosticReasonConstruction)
	}
}
