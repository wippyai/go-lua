package analysis

import (
	"context"
	"runtime"
	"sync"

	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/result"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type AnalyzeStatus uint8

const (
	AnalyzeInvalid AnalyzeStatus = iota
	AnalyzeUnsupported
	AnalyzeIncomplete
	AnalyzeComplete
)

// CompileStatus reports whether one Link was admitted to an immutable
// reusable analyzer plan. Compilation owns sealed ingress snapshots and
// Link-local substitutions. Instantiated Points and their equation topology
// are constructed once by the first Plan.Solve and shared by later solves.
type CompileStatus uint8

const (
	CompileInvalid CompileStatus = iota
	CompileUnsupported
	CompileComplete
)

// Plan is an opaque immutable analyzer plan. It retains sealed compile-time
// snapshots, Link mount substitutions, the owner-handoff ProgramArtifact bag,
// the canonical detached geometry admitted during compilation, and one
// lazily-built ordinary runtime solver. Repeated ordinary solves read that
// solver's completed immutable State.
type Plan struct {
	state     *compiledState
	workspace *Workspace
}

type compiledState struct {
	compilation composite.Compilation
	artifacts   *compiledArtifactSet
	// mounts and geometry are the detached result projection admitted during
	// Workspace compilation. Solve only reads these immutable owner values; it
	// must not reopen the Link or replay the mounted observation census.
	mounts            []programmount.MountedArtifact
	geometry          result.Geometry
	binding           *composite.ProgramBinding
	committed         *engine.CommittedProgram
	querySites        composite.SelectedQueryTable
	queryPublications []composite.QueryPublication
	sourceID          identity.ContentID
	composition       snapshot.Snapshot
	// contextDirectory is copied from Link only after the Link-lifetime
	// composition prefix has published successfully. Runtime construction then
	// consumes this exact sealed value; it never rebuilds contexts from Module.
	contextDirectory executioncontext.Directory
	selectColumn     snapshot.Axis[identity.ContentID, channelselect.CaseFact]
	selectSites      []anadiag.SelectSite
	selectHandlers   []selectapply.Handler
	vocabulary       structure.Table
	declarations     schemadiag.Table
	collections      composite.DiagnosticCollections
	admitted         bool
	runtimeOnce      sync.Once
	runtimeOK        bool
	runtimeDetail    engine.ProgramAssembleRefusal
	runtimeStage     anadiag.AnalyzeDiagnosticAssembleStage
	runtimeAdmission composite.AdmissionFailure
	ordinaryOnce     sync.Once
	ordinary         *engine.Solver
	ordinaryOK       bool
	lifecycleMu      sync.Mutex
	lifecycleCond    *sync.Cond
	leases           uint64
	closing          bool
	closed           bool
	releaseOnce      sync.Once
}

// PlacementSchema returns the exact Link-bound authority that encoded this
// plan's Placement query results. Callers may retain it while consuming the
// detached result, but cannot recover or substitute it from payload bytes.
func (plan *Plan) PlacementSchema() (placementdomain.Schema, bool) {
	if plan == nil || plan.state == nil || plan.state.binding == nil {
		return placementdomain.Schema{}, false
	}
	return plan.state.binding.PlacementSchema()
}

func Compile(source *link.Link) (*Plan, CompileStatus) {
	plan, status, _ := newWorkspace(true).compileWithDiagnostics(source)
	return plan, status
}

// CompileWithDiagnostics compiles one Link and reports the exact closed
// construction boundary on failure. It shares Compile's production path;
// diagnostics are scalar-only and cannot alter admission or topology.
func CompileWithDiagnostics(source *link.Link) (*Plan, CompileStatus, anadiag.AnalyzeDiagnostics) {
	return newWorkspace(true).compileWithDiagnostics(source)
}

// Compile compiles one Link in this Workspace. Equal immutable Program
// products are reused across this Workspace's Compile calls until Close.
func (workspace *Workspace) Compile(source *link.Link) (*Plan, CompileStatus) {
	plan, status, _ := workspace.compileWithDiagnostics(source)
	return plan, status
}

// CompileWithDiagnostics is Workspace.Compile with its closed construction
// boundary. Diagnostics do not change product identity or cache admission.
func (workspace *Workspace) CompileWithDiagnostics(source *link.Link) (*Plan, CompileStatus, anadiag.AnalyzeDiagnostics) {
	return workspace.compileWithDiagnostics(source)
}

func (workspace *Workspace) compileWithDiagnostics(source *link.Link) (*Plan, CompileStatus, anadiag.AnalyzeDiagnostics) {
	var diagnostics anadiag.AnalyzeDiagnostics
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseSetup)
	products, workspaceOK := workspace.beginCompile()
	if !workspaceOK {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonInvalidPlan)
		return nil, CompileInvalid, diagnostics
	}
	planned := false
	defer func() { workspace.finishCompile(planned) }()
	if source == nil || !source.ContentID().Available() {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonInvalidPlan)
		return nil, CompileInvalid, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseItemIssuance)
	compilation := workspace.compilation
	if !compilation.Available() {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureProgramSchema
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	vocabulary, vocabularyOK := compilation.Structure()
	declarations, declarationsOK := compilation.Diagnostics()
	collections, collectionsOK := compilation.DiagnosticCollections()
	if !vocabularyOK || !declarationsOK || !collectionsOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureProgramSchema
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	artifacts, artifactCompileFailure, artifactsOK := compileProgramArtifacts(products, source, compilation)
	if !artifactsOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureArtifacts
		// The artifact compiler publishes its refusal as a compile stage, row
		// family, and row. Recovering it here is what makes an artifacts
		// refusal name the act it stopped on instead of only its item family.
		diagnostics.ArtifactCompile = artifactCompileFailure
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	values, valuesOK := result.Coordinates(source)
	if !valuesOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureValueCoordinates
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	observations, observationsOK := artifacts.observationCensus()
	mounts := artifacts.mounts
	mountsOK := len(mounts) != 0
	geometry, resultOK := result.Geometry{}, false
	if mountsOK && observationsOK {
		geometry, resultOK = result.Project(source.ContentID(), mounts, values, observations)
	}
	if !resultOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureResultGeometry
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state := &compiledState{
		compilation: compilation, artifacts: artifacts, mounts: mounts, geometry: geometry,
		sourceID: source.ContentID(), vocabulary: vocabulary, declarations: declarations, collections: collections,
	}
	state.lifecycleCond = sync.NewCond(&state.lifecycleMu)
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseTopology)
	if !state.admit() {
		state.release()
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseAssemble)
	binding, bindingFailure, mountFailure, bindFailure := state.newProgramBinding(source, compilation)
	diagnostics.Binding = bindingFailure
	diagnostics.AllocationCatalog = bindFailure.Allocation
	// A per-rule verdict names the rule; the binder pass it rejected in is the
	// other half of that rule's evidence and travels here beside it.
	diagnostics.BindingRuleStage = bindFailure.RuleStage
	// The mount phase's verdict carries the rejecting domain's own evidence
	// erased. Recovering it at the value schema's own failure type is this
	// projection's job; a verdict from another axis carries no value evidence
	// and leaves the field absent.
	diagnostics.ValueSeal, _ = composite.MountRejection[valuedomain.SealFailure](mountFailure)
	// Recovering the pack axis's own rejection evidence follows the same rule:
	// a mount rejected outside pack leaves this field absent too.
	diagnostics.PackSeal, _ = composite.MountRejection[packowner.MountRejection](mountFailure)
	// The axis-authority verdict names one rejection; which axis raised it is
	// the sealed table's identity and travels here beside it.
	diagnostics.Axis = mountFailure.Axis
	diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageBinding
	if bindingFailure != anadiag.ProgramBindingFailureNone || binding == nil || binding.SchemaBinding() == nil || !binding.SchemaBinding().Sealed() {
		state.release()
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state.binding = binding
	contextDirectory := source.ContextDirectory()
	// The composition publication runs past a sealed binding, so its refusal
	// is its own assemble boundary. It names the step it refused at and the
	// column that step is about; leaving it at the binding stage would report
	// a binding verdict no binder produced.
	compositionFailure, compositionAxis := state.publishComposition(source.Module(), contextDirectory)
	if compositionFailure != anadiag.AnalyzeDiagnosticCompositionFailureNone {
		state.release()
		diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageComposition
		diagnostics.Composition = compositionFailure
		diagnostics.CompositionAxis = compositionAxis
		diagnostics.Axis = composite.DiagnosticAxisForKey(compilation, compositionAxis)
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state.contextDirectory = contextDirectory
	state.admitted = true
	plan := &Plan{state: state, workspace: workspace}
	runtime.SetFinalizer(plan, func(value *Plan) { _ = value.Close() })
	planned = true
	// No runtime Point, candidate, demand, or WTO authority exists yet. The
	// first Solve owns the sole transition from this cold Plan to an immutable
	// shared runtime topology.
	diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageBinding
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseComplete)
	return plan, CompileComplete, diagnostics
}

// Solve executes or reuses the Plan's one ordinary runtime transaction and
// returns only its detached public projection. A Plan may be solved repeatedly
// and concurrently; Engine serializes the first execution and publishes one
// immutable completed State to every later caller.
func (plan *Plan) Solve(ctx context.Context) (*result.Result, AnalyzeStatus) {
	result, _, status, _ := plan.solveWithPolicy(ctx, engine.SolveDiagnosticOptions{}, nil, true)
	return result, status
}

// SolveWithReport leaves inference untouched. Enabled rules collect only from
// reusable artifact subjects and observations already produced by the shared
// solve; policy selection never adds an Engine query or changes Result identity.
func (plan *Plan) SolveWithReport(ctx context.Context, options engine.SolveDiagnosticOptions, policy anadiag.DiagnosticPolicy) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	if plan == nil || plan.state == nil || !policy.Valid(plan.state.declarations) {
		return nil, nil, AnalyzeInvalid, anadiag.AnalyzeDiagnostics{Phase: anadiag.AnalyzeDiagnosticPhaseSetup, Reason: anadiag.AnalyzeDiagnosticReasonInvalidOptions}
	}
	return plan.solveWithPolicy(ctx, options, &policy, false)
}

// SolveWithDiagnostics executes one fresh source transaction and returns its
// detached analysis phase/reason envelope plus optional engine evidence. A
// zero presentation selection follows the ordinary solver semantics.
func (plan *Plan) SolveWithDiagnostics(ctx context.Context, options engine.SolveDiagnosticOptions) (*result.Result, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	result, _, status, diagnostics := plan.solveWithPolicy(ctx, options, nil, false)
	return result, status, diagnostics
}

func (plan *Plan) solveWithPolicy(ctx context.Context, options engine.SolveDiagnosticOptions, policy *anadiag.DiagnosticPolicy, reuseOrdinary bool) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	if ctx == nil {
		return nil, nil, AnalyzeInvalid, anadiag.AnalyzeDiagnostics{Phase: anadiag.AnalyzeDiagnosticPhaseSetup, Reason: anadiag.AnalyzeDiagnosticReasonInvalidPlan}
	}
	state, leased := plan.acquire()
	if !leased {
		return nil, nil, AnalyzeInvalid, anadiag.AnalyzeDiagnostics{Phase: anadiag.AnalyzeDiagnosticPhaseSetup, Reason: anadiag.AnalyzeDiagnosticReasonInvalidPlan}
	}
	defer state.releaseLease()
	if !options.Valid() {
		return nil, nil, AnalyzeInvalid, anadiag.AnalyzeDiagnostics{Phase: anadiag.AnalyzeDiagnosticPhaseSetup, Reason: anadiag.AnalyzeDiagnosticReasonInvalidOptions}
	}
	var diagnostics anadiag.AnalyzeDiagnostics
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseAssemble)
	binding := state.binding
	if binding == nil {
		diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageBinding
		diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseSolve)
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	topologyDiagnostic, topologyStage, topologyAdmission, topologyOK := state.instantiateRuntimeTopology()
	if !topologyOK {
		diagnostics.AssembleStage = topologyStage
		diagnostics.Rule = topologyAdmission.Rule
		diagnostics.EnterAdmission(topologyAdmission)
		diagnostics.EnterProgramAssemble(topologyDiagnostic, topologyAdmission.Rule)
		diagnostics.FailCurrentPhase()
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	if state.committed == nil || state.querySites.Count() == 0 {
		diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageRuntime
		diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseSolve)
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseSolve)
	diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageRuntime
	var solver *engine.Solver
	var queryPublications []composite.QueryPublication
	var compiled bool
	if reuseOrdinary && policy == nil && options == (engine.SolveDiagnosticOptions{}) {
		solver, compiled = state.ordinaryRuntimeSolver()
		queryPublications = state.queryPublications
	} else {
		solver, queryPublications, diagnostics.ObservationAttach, compiled = state.buildRuntimeSolver(policy)
	}
	if !compiled || solver == nil || len(queryPublications) == 0 {
		// Runtime binding ends at either the observation attach path or the
		// program constructor. The constructor names its own boundary, so a
		// construction refusal localizes past the generic runtime stage.
		diagnostics.EnterConstruction(diagnostics.ObservationAttach)
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	stateResult, solveStatus, engineDiagnostics := solver.SolveWithDiagnostics(ctx, options)
	diagnostics.Engine = engineDiagnostics
	if failure := engineDiagnostics.Failure; failure.Available() {
		// Rule-slot capabilities are intentionally opaque at this boundary;
		// domain diagnostics are classified while artifact rows are attached.
		diagnostics.Rule = anadiag.AnalyzeDiagnosticRuleUnknown
	}
	diagnostics.AssembleStage = anadiag.AnalyzeDiagnosticAssembleStageSolve
	switch solveStatus {
	case engine.SolveCanceled:
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineCanceled)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolvePanicked:
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEnginePanicked)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveIncomplete:
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveInvalid:
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveComplete:
		if stateResult == nil {
			diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
			return nil, nil, AnalyzeIncomplete, diagnostics
		}
	default:
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseObservation)
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseDetach)
	queriesPublished := len(queryPublications) == state.querySites.Count() && len(queryPublications) > 0
	sealed, snapshotPublished := solver.PublishedSnapshot(stateResult)
	published := sealed.Snapshot()
	queryRead, queryOpened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.QueryFamily())
	observationRead, observationOpened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.ObservationFamily())
	if !queriesPublished || !snapshotPublished || !queryOpened || !observationOpened {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	geometry := state.geometry
	if !geometry.Valid() {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	if len(state.mounts) == 0 {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	projection, detached := result.Detach(state.compilation, geometry, state.mounts, policy, queryPublications, &published, queryRead, observationRead, anadiag.ChannelSelectInput{
		Published: &state.composition,
		Column:    state.selectColumn,
		Sites:     state.selectSites,
		Handlers:  state.selectHandlers,
	}, state.vocabulary, state.declarations, state.collections, state.contextDirectory)
	if !detached || projection == nil || projection.Result == nil {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseComplete)
	return projection.Result, projection.Report, AnalyzeComplete, diagnostics
}

// ordinaryRuntimeSolver owns the single program construction used by ordinary
// Plan.Solve calls. Construction is independent of the caller context, so a
// canceled first call cannot poison the immutable compiler or prevent a later
// caller from completing it. Solver itself serializes execution and publishes
// exactly one completed State per accepted runtime revision.
func (state *compiledState) ordinaryRuntimeSolver() (*engine.Solver, bool) {
	if state == nil {
		return nil, false
	}
	state.ordinaryOnce.Do(func() {
		var publications []composite.QueryPublication
		var failure engine.SolveFailure
		state.ordinary, publications, failure, state.ordinaryOK = state.buildRuntimeSolver(nil)
		state.ordinaryOK = state.ordinaryOK && len(publications) == state.querySites.Count() && !failure.Available()
		if state.ordinaryOK {
			state.queryPublications = publications
		}
		if !state.ordinaryOK {
			state.ordinary = nil
			state.queryPublications = nil
		}
	})
	return state.ordinary, state.ordinaryOK
}

// Runtime publication retains its established phase ordinal. The bootstrap
// witness was already consumed by committed-program construction and is not
// re-derived at solve time.
const runtimeSealPhasePublications uint64 = 2

// buildRuntimeSolver is the sole runtime binding path. Ordinary solves retain
// its result through ordinaryRuntimeSolver; diagnostic-policy solves invoke it
// afresh because their observation inventory is explicitly flag-controlled.
//
// The committed program carries every member and query row its construction
// declared, so the only inventory this pass states is the observation set the
// policy selected; the seal binds the whole runtime from both.
func (state *compiledState) buildRuntimeSolver(policy *anadiag.DiagnosticPolicy) (*engine.Solver, []composite.QueryPublication, engine.SolveFailure, bool) {
	if state == nil || state.binding == nil || state.binding.SchemaBinding() == nil || state.committed == nil || state.querySites.Count() == 0 || state.artifacts == nil {
		return nil, nil, engine.SolveFailure{}, false
	}
	geometry := state.geometry
	if !geometry.Valid() {
		return nil, nil, engine.SolveFailure{}, false
	}
	binding := state.binding
	publications, published := binding.QueryPublications(state.committed, state.querySites)
	if !published || len(publications) != state.querySites.Count() {
		return nil, nil, engine.ProgramSealFailure(runtimeSealPhasePublications), false
	}
	observations, observationFailure, observed := anadiag.ValueObservations(state.committed, binding, state.contextDirectory, geometry.BranchObservations, geometry.ConformanceObservations)
	if !observed {
		return nil, nil, observationFailure, false
	}
	callObservations, callObserved := binding.CallCalleeSetObservations(state.committed, state.mounts, state.contextDirectory)
	if !callObserved {
		return nil, nil, engine.ObservationSealArguments(), false
	}
	observations = append(observations, callObservations...)
	effectObservations, effectObserved := binding.EffectPublicationObservations(state.committed, state.mounts, state.contextDirectory)
	if !effectObserved {
		return nil, nil, engine.ObservationSealArguments(), false
	}
	observations = append(observations, effectObservations...)
	solver, sealFailure, sealed := state.committed.Seal(observations)
	if !sealed || solver == nil {
		// The seal reports which stage refused. Only fall back to the
		// observation boundary when it named none.
		if sealFailure.Available() {
			return nil, nil, sealFailure, false
		}
		return nil, nil, observationFailure, false
	}
	return solver, publications, observationFailure, true
}

// SourceID is the content fence of the Link compiled into this plan.
func (plan *Plan) SourceID() identity.ContentID {
	state, leased := plan.acquire()
	if !leased {
		return identity.ContentID{}
	}
	defer state.releaseLease()
	return state.sourceID
}

// Close releases this Plan's assembled topology and domain bindings, then
// returns its product lease to the owning Workspace. An explicit Workspace may
// reuse those immutable products until Workspace.Close; a convenience Plan's
// private Workspace releases them immediately. Close is terminal; the
// finalizer is only a leak safety net.
func (plan *Plan) Close() bool {
	if plan == nil || plan.state == nil {
		return false
	}
	state := plan.state
	state.lifecycleMu.Lock()
	if state.closing || state.closed {
		state.lifecycleMu.Unlock()
		return false
	}
	state.closing = true
	for state.leases != 0 {
		state.lifecycleCond.Wait()
	}
	state.closed = true
	state.lifecycleMu.Unlock()
	state.release()
	workspace := plan.workspace
	plan.workspace = nil
	if workspace != nil {
		workspace.releasePlan()
	}
	runtime.SetFinalizer(plan, nil)
	return true
}

func (plan *Plan) acquire() (*compiledState, bool) {
	if plan == nil || plan.state == nil {
		return nil, false
	}
	state := plan.state
	state.lifecycleMu.Lock()
	defer state.lifecycleMu.Unlock()
	if state.closing || state.closed || !state.admitted || state.artifacts == nil ||
		state.binding == nil || state.binding.SchemaBinding() == nil || !state.binding.SchemaBinding().Sealed() || !state.sourceID.Available() {
		return nil, false
	}
	state.leases++
	return state, true
}

func (state *compiledState) releaseLease() {
	if state == nil {
		return
	}
	state.lifecycleMu.Lock()
	if state.leases > 0 {
		state.leases--
	}
	if state.leases == 0 {
		state.lifecycleCond.Broadcast()
	}
	state.lifecycleMu.Unlock()
}

func (state *compiledState) release() {
	if state == nil {
		return
	}
	state.lifecycleMu.Lock()
	state.closed = true
	state.closing = true
	state.lifecycleMu.Unlock()
	state.releaseOnce.Do(func() {
		// Workspace owns reusable immutable products. A closed Plan must not keep
		// a second strong mount set or any Link-local authority alive.
		state.artifacts = nil
		state.compilation = composite.Compilation{}
		state.committed = nil
		state.querySites = composite.SelectedQueryTable{}
		state.queryPublications = nil
		state.ordinary = nil
		state.ordinaryOK = false
		state.mounts = nil
		state.geometry = result.Geometry{}
		state.binding = nil
		state.contextDirectory = executioncontext.Directory{}
		state.composition = snapshot.Snapshot{}
		state.selectColumn = snapshot.Axis[identity.ContentID, channelselect.CaseFact]{}
		state.selectSites = nil
		state.selectHandlers = nil
		state.vocabulary = structure.Table{}
		state.declarations = schemadiag.Table{}
		state.collections = composite.DiagnosticCollections{}
		state.runtimeDetail = engine.ProgramAssembleRefusal{}
		state.runtimeStage = anadiag.AnalyzeDiagnosticAssembleStageNone
		state.runtimeAdmission = composite.AdmissionFailure{}
		state.runtimeOK = false
		state.admitted = false
	})
}

func (state *compiledState) admit() bool {
	if state == nil || state.artifacts == nil || !state.sourceID.Available() {
		return false
	}
	if len(state.artifacts.mounts) == 0 || len(state.artifacts.products) == 0 {
		return false
	}
	for _, mount := range state.artifacts.mounts {
		if !mount.Available() {
			return false
		}
	}
	return state.geometry.Valid()
}

func Analyze(ctx context.Context, source *link.Link) (*result.Result, AnalyzeStatus) {
	return newWorkspace(true).analyze(ctx, source)
}

// Analyze compiles and solves one Link while retaining reusable immutable
// products in this Workspace for later calls. The per-run Plan is always
// closed before Analyze returns.
func (workspace *Workspace) Analyze(ctx context.Context, source *link.Link) (*result.Result, AnalyzeStatus) {
	return workspace.analyze(ctx, source)
}

func (workspace *Workspace) analyze(ctx context.Context, source *link.Link) (*result.Result, AnalyzeStatus) {
	if ctx == nil || source == nil || !source.ContentID().Available() {
		return nil, AnalyzeInvalid
	}
	plan, status := workspace.Compile(source)
	switch status {
	case CompileInvalid:
		return nil, AnalyzeInvalid
	case CompileUnsupported:
		return nil, AnalyzeUnsupported
	case CompileComplete:
		defer plan.Close()
		return plan.Solve(ctx)
	default:
		return nil, AnalyzeUnsupported
	}
}

func (state *compiledState) assembleCommittedProgram() (*engine.CommittedProgram, composite.SelectedQueryTable, anadiag.AnalyzeDiagnosticAssembleStage, composite.AdmissionFailure, engine.ProgramAssembleRefusal, bool) {
	if state == nil || state.artifacts == nil || state.binding == nil || state.binding.SchemaBinding() == nil || !state.binding.SchemaBinding().Sealed() {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageNone, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	binding := state.binding
	witness, witnessOK := linkBootstrapWitness(state, binding)
	if !witnessOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageBinding, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	inputs := make([]engine.MountedProgramArtifact, 0, len(state.artifacts.mounts))
	rolesByArtifact := make(map[identity.ContentID][]engine.MountedProgramRole, len(state.artifacts.mounts))
	factorsByArtifact := make(map[identity.ContentID][]engine.MountedProgramFactor, len(state.artifacts.mounts))
	for _, mount := range state.artifacts.mounts {
		if !mount.Available() {
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageMount, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
		}
		artifactID := mount.Snapshot.ArtifactID()
		roles, have := rolesByArtifact[artifactID]
		factors := factorsByArtifact[artifactID]
		if !have {
			product, productOK := state.artifacts.products[mount.ProgramID]
			if !productOK {
				return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageMount, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
			}
			boundRoles, boundFactors, boundOK := mountedProgramBindings(product.Bindings, binding)
			if !boundOK {
				return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageMount, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
			}
			roles, factors = boundRoles, boundFactors
			rolesByArtifact[artifactID] = roles
			factorsByArtifact[artifactID] = factors
		}
		product, productOK := state.artifacts.products[mount.ProgramID]
		if !productOK || product.Template == nil || !product.Template.Available() {
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageMount, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
		}
		// The mount carries the publication its issued candidate ordinals
		// address. A Program that publishes no readable cold state cannot back
		// a placement that names one, so the mount refuses rather than
		// admitting an ordinal with nothing behind it.
		state, stateOK := mount.Snapshot.Program().ColdState()
		if !stateOK {
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageMount, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
		}
		inputs = append(inputs, engine.MountedProgramArtifact{Template: product.Template, Roles: roles, Factors: factors, Module: mount.ModuleKey, State: state})
	}
	sealed := state.artifacts.mounts
	sealedOK := len(sealed) != 0
	rules := binding.Rules()
	if !sealedOK || rules == nil {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageBinding, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	sites, queryOK := composite.SelectedQuerySites(state.compilation, sealed, state.contextDirectory)
	if !queryOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageQueryPlan, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	linkAdmissions, linkOK := rules.LinkAdmissions()
	if !linkOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageBootstrapRules, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	mountedPoint, mountedPointOK := rules.MountedPointAdmissions()
	if !mountedPointOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageArtifactRules, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	mounted, activations, admissionFailure := rules.MountedAdmissions(sealed, state.contextDirectory)
	if admissionFailure.Available() {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageArtifactRules, admissionFailure, engine.ProgramAssembleRefusal{}, false
	}
	queries, queriesOK := binding.QueryAdmissions(sites)
	if !queriesOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageQueryRows, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	pointTransitions, pointTransitionsOK := state.pointTransitionAdmissions()
	if !pointTransitionsOK {
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageCommit, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, false
	}
	admission := engine.MountedProgramAdmission{
		Link:         linkAdmissions,
		Mounted:      mounted,
		MountedPoint: mountedPoint,
		Activation:   activations,
		Queries:      queries,
	}
	program, refusal, committed := engine.ConstructProgram(engine.ProgramDeclaration{
		Binding:          binding.SchemaBinding(),
		Mounts:           inputs,
		Bootstrap:        witness,
		Contexts:         state.contextDirectory,
		Admission:        admission,
		PointTransitions: pointTransitions,
	})
	if !committed {
		if refusal.Lowered() {
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageLowering, composite.AdmissionFailure{}, refusal, false
		}
		switch refusal.Stage() {
		case engine.ProgramAdmissionLink:
			linkRule := anadiag.AnalyzeDiagnosticRuleUnknown
			if role, roleOK := refusal.LinkRole(); roleOK {
				linkRule = diagnosticRuleForLinkRole(binding, role)
			}
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageBootstrapRules, composite.RefusedAdmissionRule(composite.AdmissionStageConstruction, linkRule), refusal, false
		case engine.ProgramAdmissionMounted:
			mountedRule := anadiag.AnalyzeDiagnosticRuleUnknown
			if role, roleOK := refusal.MountedRole(); roleOK {
				mountedRule = diagnosticRuleForMountedRole(binding, role)
			}
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageArtifactRules, composite.RefusedAdmissionRule(composite.AdmissionStageConstruction, mountedRule), refusal, false
		case engine.ProgramAdmissionQuery:
			return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageQueryRows, composite.AdmissionFailure{}, refusal, false
		case engine.ProgramAdmissionSeal:
			failedRule := anadiag.AnalyzeDiagnosticRuleUnknown
			failedStage := anadiag.AnalyzeDiagnosticAssembleStageSourceSeal
			if _, artifactRows := refusal.ArtifactRowOrdinal(); artifactRows {
				failedStage = anadiag.AnalyzeDiagnosticAssembleStageArtifactRows
			} else if role, roleOK := refusal.MountedRole(); roleOK {
				failedRule = diagnosticRuleForMountedRole(binding, role)
			} else if role, roleOK := refusal.LinkRole(); roleOK {
				failedRule = diagnosticRuleForLinkRole(binding, role)
			}
			return nil, composite.SelectedQueryTable{}, failedStage, composite.RefusedAdmissionRule(composite.AdmissionStageConstruction, failedRule), refusal, false
		}
		return nil, composite.SelectedQueryTable{}, anadiag.AnalyzeDiagnosticAssembleStageCommit, composite.AdmissionFailure{}, refusal, false
	}
	return program, sites, anadiag.AnalyzeDiagnosticAssembleStageCommit, composite.AdmissionFailure{}, engine.ProgramAssembleRefusal{}, true
}

// pointTransitionAdmissions reads the exact module-call and initialization
// rows back from the sealed Link composition. The Snapshot remains the sole
// row authority: compiledState does not retain a parallel transition slice,
// and the engine receives only pairs whose GenerationID join is present in
// the published typed columns.
func (state *compiledState) pointTransitionAdmissions() ([]engine.ProgramPointTransitionAdmission, bool) {
	if state == nil || !state.composition.Published() {
		return nil, false
	}
	publication, publicationOK := state.compilation.Publication()
	transitionAxis, transitionAxisOK := analysiscatalog.ProjectAxis[identity.ContentID, modulecomposition.ModuleCallTransition](publication, modulecomposition.ModuleCallTransitionOutputKey)
	generationAxis, generationAxisOK := analysiscatalog.ProjectAxis[identity.ContentID, modulecomposition.InitGeneration](publication, modulecomposition.GenerationOutputKey)
	if !publicationOK || !transitionAxisOK || !generationAxisOK {
		return nil, false
	}
	transitionCount, transitionsPublished := snapshot.MemberCountAtAxis(&state.composition, transitionAxis)
	generationCount, generationsPublished := snapshot.MemberCountAtAxis(&state.composition, generationAxis)
	if !transitionsPublished || !generationsPublished || transitionCount != generationCount {
		return nil, false
	}
	rows := make([]engine.ProgramPointTransitionAdmission, 0, transitionCount)
	seenGenerations := make(map[identity.ContentID]struct{}, generationCount)
	for index := 0; index < transitionCount; index++ {
		transitionID, memberOK := snapshot.MemberAtAxis(&state.composition, transitionAxis, index)
		transition, transitionOK := modulecomposition.ModuleCallTransitionAt(&state.composition, transitionAxis, transitionID)
		generation, generationOK := modulecomposition.GenerationAt(&state.composition, generationAxis, transition.GenerationID())
		if !memberOK || !transitionOK || !generationOK || transition.ID() != transitionID || generation.ID() != transition.GenerationID() {
			return nil, false
		}
		if _, duplicate := seenGenerations[generation.ID()]; duplicate {
			return nil, false
		}
		seenGenerations[generation.ID()] = struct{}{}
		rows = append(rows, engine.ProgramPointTransitionAdmission{Transition: transition, Generation: generation})
	}
	for index := 0; index < generationCount; index++ {
		generationID, memberOK := snapshot.MemberAtAxis(&state.composition, generationAxis, index)
		if _, consumed := seenGenerations[generationID]; !memberOK || !consumed {
			return nil, false
		}
	}
	return rows, true
}

func (state *compiledState) instantiateRuntimeTopology() (engine.ProgramAssembleRefusal, anadiag.AnalyzeDiagnosticAssembleStage, composite.AdmissionFailure, bool) {
	if state == nil {
		return engine.ProgramAssembleRefusal{}, anadiag.AnalyzeDiagnosticAssembleStageCommit, composite.AdmissionFailure{}, false
	}
	state.runtimeOnce.Do(func() {
		state.runtimeDetail, state.runtimeStage, state.runtimeAdmission, state.runtimeOK = state.buildRuntimeTopologyWithDiagnostic()
	})
	return state.runtimeDetail, state.runtimeStage, state.runtimeAdmission, state.runtimeOK
}

func (state *compiledState) buildRuntimeTopologyWithDiagnostic() (engine.ProgramAssembleRefusal, anadiag.AnalyzeDiagnosticAssembleStage, composite.AdmissionFailure, bool) {
	if state == nil || state.committed != nil {
		return engine.ProgramAssembleRefusal{}, anadiag.AnalyzeDiagnosticAssembleStageCommit, composite.AdmissionFailure{}, state != nil && state.committed != nil
	}
	program, sites, stage, admission, diagnostic, ok := state.assembleCommittedProgram()
	if !ok || program == nil {
		return diagnostic, stage, admission, false
	}
	state.committed = program
	state.querySites = sites
	return engine.ProgramAssembleRefusal{}, stage, admission, true
}
