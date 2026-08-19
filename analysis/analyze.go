package analysis

import (
	"context"
	"runtime"
	"sync"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
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
// and one lazily-built ordinary runtime solver. Repeated ordinary solves read
// that solver's completed immutable State.
type Plan struct {
	state *compiledState
}

type compiledState struct {
	artifacts         *compiledArtifactSet
	coordinates       []compiledValueCoordinate
	receipt           composite.Compilation
	binding           *composite.ProgramBinding
	committed         committedProgramGraph
	querySites        []composite.QuerySite
	queryPublications []composite.QueryPublication
	sourceID          identity.ContentID
	composition       snapshot.Snapshot
	selectSites       []anadiag.SelectSite
	selectHandlers    []selectapply.Handler
	admitted          bool
	runtimeOnce       sync.Once
	runtimeOK         bool
	runtimeDetail     assembleDiagnostic
	ordinaryOnce      sync.Once
	ordinary          *engine.Solver
	ordinaryOK        bool
	lifecycleMu       sync.Mutex
	lifecycleCond     *sync.Cond
	leases            uint64
	closing           bool
	closed            bool
	releaseOnce       sync.Once
}

func Compile(source *link.Link) (*Plan, CompileStatus) {
	plan, status, _ := CompileWithDiagnostics(source)
	return plan, status
}

// CompileWithDiagnostics compiles one Link and reports the exact closed
// construction boundary on failure. It shares Compile's production path;
// diagnostics are scalar-only and cannot alter admission or topology.
func CompileWithDiagnostics(source *link.Link) (*Plan, CompileStatus, anadiag.AnalyzeDiagnostics) {
	var diagnostics anadiag.AnalyzeDiagnostics
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseSetup)
	if source == nil || !source.ContentID().Available() {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonInvalidPlan)
		return nil, CompileInvalid, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseItemIssuance)
	receipt, receiptOK := composite.Global()
	if !receiptOK || !receipt.Available() {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureProgramSchema
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	artifacts, artifactsOK := compileProgramArtifacts(source, receipt)
	if !artifactsOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureArtifacts
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	values, valuesOK := compileValueCoordinates(source)
	if !valuesOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureValueCoordinates
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state := &compiledState{artifacts: artifacts, coordinates: values, receipt: receipt, sourceID: source.ContentID()}
	if _, resultOK := state.resultGeometry(); !resultOK {
		diagnostics.ItemIssuance = anadiag.AnalyzeDiagnosticItemIssuanceFailureResultGeometry
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state.lifecycleCond = sync.NewCond(&state.lifecycleMu)
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseTopology)
	if !state.admit() {
		state.release()
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	diagnostics.Enter(anadiag.AnalyzeDiagnosticPhaseAssemble)
	_, binding, bindingFailure, mountFailure, bindFailure := state.newProgramBinding(source)
	diagnostics.Binding = bindingFailure
	diagnostics.AllocationCatalog = bindFailure.Allocation
	// The mount phase's verdict carries the rejecting domain's own evidence
	// erased. Recovering it at the value schema's own failure type is this
	// projection's job; a verdict from another axis carries no value evidence
	// and leaves the field absent.
	diagnostics.ValueSeal, _ = composite.MountRejection[valuedomain.SealFailure](mountFailure)
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
	if !state.publishComposition(source) {
		state.release()
		diagnostics.FailCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state.admitted = true
	plan := &Plan{state: state}
	runtime.SetFinalizer(plan, func(value *Plan) { _ = value.Close() })
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
	if !policy.Valid() {
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
	receiptDiagnostic, topologyOK := state.instantiateRuntimeTopology()
	if !topologyOK {
		applyAssembleDiagnostic(&diagnostics, receiptDiagnostic)
		diagnostics.FailCurrentPhase()
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	if state.committed.program == nil || len(state.querySites) == 0 {
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
	queriesPublished := len(queryPublications) == len(state.querySites) && len(queryPublications) > 0
	sealed, snapshotPublished := solver.PublishedSnapshot(stateResult)
	published := sealed.Snapshot()
	queryRead, queryOpened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.QueryFamily())
	observationRead, observationOpened := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.ObservationFamily())
	if !queriesPublished || !snapshotPublished || !queryOpened || !observationOpened {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	geometry, geometryOK := state.resultGeometry()
	if !geometryOK {
		diagnostics.Fail(anadiag.AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	projection, detached := result.Detach(geometry, resultMounts(state.artifacts.mounts), binding.ValueSchema(), policy, queryPublications, &published, queryRead, observationRead, anadiag.ChannelSelectInput{
		Published: &state.composition,
		Sites:     state.selectSites,
		Handlers:  state.selectHandlers,
	})
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
		state.ordinaryOK = state.ordinaryOK && len(publications) == len(state.querySites) && !failure.Available()
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

// Runtime seal phases run in this order against one committed program. The
// ordinal names the phase inside the engine's compile-family site identity, so
// an incomplete runtime binding reports which phase rejected.
const (
	runtimeSealPhaseWitness uint64 = iota + 1
	runtimeSealPhasePublications
)

// buildRuntimeSolver is the sole runtime binding path. Ordinary solves retain
// its result through ordinaryRuntimeSolver; diagnostic-policy solves invoke it
// afresh because their observation inventory is explicitly flag-controlled.
//
// The committed program carries every member and query row its construction
// declared, so the only inventory this pass states is the observation set the
// policy selected; the seal binds the whole runtime from both.
func (state *compiledState) buildRuntimeSolver(policy *anadiag.DiagnosticPolicy) (*engine.Solver, []composite.QueryPublication, engine.SolveFailure, bool) {
	if state == nil || state.binding == nil || state.binding.SchemaBinding() == nil || state.committed.program == nil || len(state.querySites) == 0 || state.artifacts == nil {
		return nil, nil, engine.SolveFailure{}, false
	}
	geometry, geometryOK := state.resultGeometry()
	if !geometryOK {
		return nil, nil, engine.SolveFailure{}, false
	}
	binding := state.binding
	_, witnessOK := linkBootstrapWitness(state, binding)
	if !witnessOK {
		return nil, nil, engine.ProgramSealFailure(runtimeSealPhaseWitness), false
	}
	publications, published := binding.QueryPublications(state.committed.program, state.querySites)
	if !published || len(publications) != len(state.querySites) {
		return nil, nil, engine.ProgramSealFailure(runtimeSealPhasePublications), false
	}
	observations, observationFailure, observed := anadiag.BranchValueObservations(state.committed.program, binding, geometry.BranchObservations)
	if !observed {
		return nil, nil, observationFailure, false
	}
	solver, sealFailure, sealed := state.committed.program.Seal(observations)
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

// Close releases this Plan's assembled topology and domain receipts. The
// compile-time snapshot, template, and owner-handoff bag remain in the
// content-addressed cache: closing a Plan must not force a later equivalent
// Link to recompile or Lower them. It is terminal; the finalizer is only a
// leak safety net.
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
	if state.closing || state.closed || !state.admitted || state.artifacts == nil || !state.receipt.Available() ||
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
		// The global content-addressed cache owns successful immutable artifacts.
		// A closed Plan must not keep a second strong mount set alive.
		state.artifacts = nil
		state.committed = committedProgramGraph{}
		state.querySites = nil
		state.queryPublications = nil
		state.ordinary = nil
		state.coordinates = nil
		state.binding = nil
		state.admitted = false
	})
}

func resultMounts(mounts []mountedProgramArtifact) []result.Mount {
	out := make([]result.Mount, len(mounts))
	for index, mount := range mounts {
		out[index] = result.Mount{Snapshot: mount.snapshot, ModuleKey: mount.moduleKey, Program: mount.program}
	}
	return out
}

func resultCoordinates(coordinates []compiledValueCoordinate) ([]result.ValueCoordinate, bool) {
	out := make([]result.ValueCoordinate, len(coordinates))
	for index, coordinate := range coordinates {
		row, ok := result.NewValueCoordinate(coordinate.id, coordinate.mount)
		if !ok {
			return nil, false
		}
		out[index] = row
	}
	return out, true
}

func (state *compiledState) resultGeometry() (result.Geometry, bool) {
	if state == nil || state.artifacts == nil {
		return result.Geometry{}, false
	}
	observations, observationsOK := state.artifacts.observationCensus(state.coordinates)
	if !observationsOK {
		return result.Geometry{}, false
	}
	coordinates, coordinatesOK := resultCoordinates(state.coordinates)
	if !coordinatesOK {
		return result.Geometry{}, false
	}
	return result.Project(state.sourceID, resultMounts(state.artifacts.mounts), coordinates, observations)
}

func (state *compiledState) admit() bool {
	if state == nil || state.artifacts == nil || !state.receipt.Available() || !state.sourceID.Available() {
		return false
	}
	if len(state.artifacts.mounts) == 0 || len(state.artifacts.byProgram) == 0 {
		return false
	}
	for _, mount := range state.artifacts.mounts {
		if !mount.valid() {
			return false
		}
	}
	geometry, geometryOK := state.resultGeometry()
	return geometryOK && geometry.Valid()
}

func Analyze(ctx context.Context, source *link.Link) (*result.Result, AnalyzeStatus) {
	if ctx == nil || source == nil || !source.ContentID().Available() {
		return nil, AnalyzeInvalid
	}
	plan, status := Compile(source)
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

// committedProgramGraph is the assemble-owned committed handle. analyze
// opens the existing construction only after assemble commits the graph.
type committedProgramGraph struct {
	program *engine.CommittedProgram
}

type assembleDiagnostic struct {
	stage    anadiag.AnalyzeDiagnosticAssembleStage
	rule     anadiag.AnalyzeDiagnosticRule
	seal     engine.SolveFailure
	ordinal  uint32
	lowering engine.SolveFailure
	binding  anadiag.ProgramBindingFailure
	commit   engine.SolveFailure
	schedule uint32
}

func (state *compiledState) assembleCommittedProgram() (*engine.CommittedProgram, []composite.QuerySite, assembleDiagnostic, bool) {
	if state == nil || state.artifacts == nil || !state.receipt.Available() || state.binding == nil || state.binding.SchemaBinding() == nil || !state.binding.SchemaBinding().Sealed() {
		return nil, nil, assembleDiagnostic{}, false
	}
	binding := state.binding
	witness, witnessOK := linkBootstrapWitness(state, binding)
	if !witnessOK {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageBinding}, false
	}
	inputs := make([]engine.MountedProgramArtifact, 0, len(state.artifacts.mounts))
	rolesByArtifact := make(map[identity.ContentID][]engine.MountedProgramRole, len(state.artifacts.mounts))
	for _, mount := range state.artifacts.mounts {
		if !mount.valid() {
			return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageMount}, false
		}
		artifactID := mount.snapshot.ArtifactID()
		roles, have := rolesByArtifact[artifactID]
		if !have {
			bound, boundOK := mountedProgramRoles(mount.roles, binding)
			if !boundOK {
				return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageMount}, false
			}
			roles = bound
			rolesByArtifact[artifactID] = roles
		}
		inputs = append(inputs, engine.MountedProgramArtifact{Template: mount.template, Roles: roles, Module: mount.moduleKey})
	}
	sealed, sealedOK := linkArtifactRows(state.artifacts.mounts)
	rules := binding.Rules()
	if !sealedOK || rules == nil {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageBinding}, false
	}
	sites, queryOK := composite.SelectedQuerySites(sealed)
	if !queryOK {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageQueryPlan}, false
	}
	linkAdmissions, linkOK := rules.LinkAdmissions()
	if !linkOK {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageBootstrapRules}, false
	}
	mounted, activations, artifactRule, mountedOK := rules.MountedAdmissions(sealed)
	if !mountedOK {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageArtifactRules, rule: artifactRule}, false
	}
	queries, queriesOK := binding.QueryAdmissions(sites)
	if !queriesOK {
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageQueryRows}, false
	}
	admission := engine.MountedProgramAdmission{
		Link:       linkAdmissions,
		Mounted:    mounted,
		Activation: activations,
		Queries:    queries,
	}
	program, refusal, committed := engine.ConstructProgram(engine.ProgramDeclaration{
		Binding:   binding.SchemaBinding(),
		Mounts:    inputs,
		Bootstrap: witness,
		Admission: admission,
	})
	if !committed {
		if refusal.Lowered() {
			return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageLowering, lowering: refusal.LoweringFailure()}, false
		}
		switch refusal.Stage() {
		case engine.ProgramAdmissionLink:
			return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageBootstrapRules}, false
		case engine.ProgramAdmissionMounted:
			return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageArtifactRules}, false
		case engine.ProgramAdmissionQuery:
			return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageQueryRows}, false
		case engine.ProgramAdmissionSeal:
			failedRule := anadiag.AnalyzeDiagnosticRuleUnknown
			failedStage := anadiag.AnalyzeDiagnosticAssembleStageSourceSeal
			var failedOrdinal uint32
			if ordinal, artifactRows := refusal.ArtifactRowOrdinal(); artifactRows {
				failedStage = anadiag.AnalyzeDiagnosticAssembleStageArtifactRows
				failedOrdinal = ordinal
			} else if role, roleOK := refusal.MountedRole(); roleOK {
				failedRule = diagnosticRuleForMountedRole(binding, role)
			} else if role, roleOK := refusal.LinkRole(); roleOK {
				failedRule = diagnosticRuleForLinkRole(binding, role)
			}
			return nil, nil, assembleDiagnostic{stage: failedStage, rule: failedRule, seal: refusal.Seal(), ordinal: failedOrdinal}, false
		}
		return nil, nil, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageCommit, commit: refusal.Commit(), schedule: refusal.ScheduleRow()}, false
	}
	return program, sites, assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageCommit}, true
}

func (state *compiledState) instantiateRuntimeTopology() (assembleDiagnostic, bool) {
	if state == nil {
		return assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageCommit}, false
	}
	state.runtimeOnce.Do(func() {
		state.runtimeDetail, state.runtimeOK = state.buildRuntimeTopologyWithDiagnostic()
	})
	return state.runtimeDetail, state.runtimeOK
}

func (state *compiledState) buildRuntimeTopologyWithDiagnostic() (assembleDiagnostic, bool) {
	if state == nil || state.committed.program != nil {
		return assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageCommit}, state != nil && state.committed.program != nil
	}
	program, sites, diagnostic, ok := state.assembleCommittedProgram()
	if !ok || program == nil {
		return diagnostic, false
	}
	state.committed.program = program
	state.querySites = sites
	return assembleDiagnostic{stage: anadiag.AnalyzeDiagnosticAssembleStageCommit}, true
}

func applyAssembleDiagnostic(diagnostics *anadiag.AnalyzeDiagnostics, detail assembleDiagnostic) {
	if diagnostics == nil {
		return
	}
	diagnostics.AssembleStage = detail.stage
	diagnostics.Rule = detail.rule
	diagnostics.AssembleSeal = detail.seal
	diagnostics.AssembleOrdinal = detail.ordinal
	diagnostics.AssembleLowering = detail.lowering
	diagnostics.Binding = detail.binding
	diagnostics.AssembleCommit = detail.commit
	diagnostics.AssembleScheduleOrdinal = detail.schedule
}
