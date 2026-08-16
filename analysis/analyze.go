package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

const resultFormat uint64 = 9

type AnalyzeStatus uint8

const (
	AnalyzeInvalid AnalyzeStatus = iota
	AnalyzeUnsupported
	AnalyzeIncomplete
	AnalyzeComplete
)

// CompileStatus reports whether one Link was admitted to an immutable
// reusable analyzer plan. Compilation owns Program artifacts and Link-local
// substitutions only. Instantiated Points and their equation topology are
// constructed once by the first Plan.Solve and shared by later solves.
type CompileStatus uint8

const (
	CompileInvalid CompileStatus = iota
	CompileUnsupported
	CompileComplete
)

// Plan is an opaque immutable analyzer plan. It retains reusable sealed
// Program artifacts, their Link mount substitutions, and one lazily-built
// ordinary runtime solver. Repeated ordinary solves read that solver's
// completed immutable State instead of rebuilding or re-solving unchanged
// Program interiors.
type Plan struct {
	state *compiledState
}

type compiledState struct {
	artifacts            *compiledArtifactSet
	resultReceipt        *artifactResultReceipt
	receipt              programschema.CompilationReceipt
	binding              *programBinding
	graph                *engine.ReceiptGraph
	queryPlan            *artifactQueryPlan
	sourceID             keyspace.ContentID
	admitted             bool
	runtimeOnce          sync.Once
	runtimeOK            bool
	runtimeDetail        receiptAssemblyDiagnostic
	ordinaryOnce         sync.Once
	ordinary             *engine.Solver
	ordinaryObservations []artifactDiagnosticObservationReceipt
	ordinaryOK           bool
	lifecycleMu          sync.Mutex
	lifecycleCond        *sync.Cond
	leases               uint64
	closing              bool
	closed               bool
	releaseOnce          sync.Once
}

// Result is a detached projection of canonical body-root and query rows. It
// retains neither Link/domain/engine handles nor template classifications.
type Result struct {
	source  keyspace.ContentID
	content keyspace.ContentID
	values  []keyspace.ContentID
	bodies  []resultBody
	// native is the one sealed post-convergence publication receipt. An
	// available empty receipt is distinct from a missing producer.
	native *nativePublicationReceipt
	// placement remains nil until its typed post-convergence owner can issue a
	// solved receipt. Nil is deliberately unavailable, not an empty placement
	// result.
	placement *placementResultReceipt
	sealed    bool
}

type resultBody struct {
	id            keyspace.ContentID
	roots         []resultRoot
	valuePresence []uint64
	effectPresent bool
	effectTop     bool
	effects       []keyspace.ContentID
}

type resultRoot struct {
	id     keyspace.ContentID
	family keyspace.Family
}

type Body struct {
	owner   *Result
	ordinal uint32
}

// Root is a detached exact executable root row of one Body.
type Root struct {
	owner *Result
	body  uint32
	index uint32
}

func Compile(source *link.Link) (*Plan, CompileStatus) {
	plan, status, _ := CompileWithDiagnostics(source)
	return plan, status
}

// CompileWithDiagnostics compiles one Link and reports the exact closed
// construction boundary on failure. It shares Compile's production path;
// diagnostics are scalar-only and cannot alter admission or topology.
func CompileWithDiagnostics(source *link.Link) (*Plan, CompileStatus, AnalyzeDiagnostics) {
	var diagnostics AnalyzeDiagnostics
	diagnostics.enter(AnalyzeDiagnosticPhaseSetup)
	if source == nil || !source.ContentID().Available() {
		diagnostics.fail(AnalyzeDiagnosticReasonInvalidPlan)
		return nil, CompileInvalid, diagnostics
	}
	diagnostics.enter(AnalyzeDiagnosticPhaseItemIssuance)
	receipt, receiptOK := programschema.Global()
	if !receiptOK || !receipt.Available() {
		diagnostics.ItemIssuance = AnalyzeDiagnosticItemIssuanceFailureProgramSchema
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	artifacts, artifactsOK := compileProgramArtifacts(source, receipt)
	if !artifactsOK {
		diagnostics.ItemIssuance = AnalyzeDiagnosticItemIssuanceFailureArtifacts
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	values, valuesOK := compileValueCoordinates(source)
	if !valuesOK {
		diagnostics.ItemIssuance = AnalyzeDiagnosticItemIssuanceFailureValueCoordinates
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	diagnosticObservations, diagnosticObservationsOK := compileDiagnosticObservations(source, artifacts, values)
	if !diagnosticObservationsOK {
		diagnostics.ItemIssuance = AnalyzeDiagnosticItemIssuanceFailureDiagnosticObservations
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	resultReceipt, resultReceiptOK := compileArtifactResultReceipt(source.ContentID(), artifacts.mounts, values, diagnosticObservations)
	if !resultReceiptOK {
		diagnostics.ItemIssuance = AnalyzeDiagnosticItemIssuanceFailureResultReceipt
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state := &compiledState{artifacts: artifacts, resultReceipt: resultReceipt, receipt: receipt, sourceID: source.ContentID()}
	state.lifecycleCond = sync.NewCond(&state.lifecycleMu)
	diagnostics.enter(AnalyzeDiagnosticPhaseTopology)
	if !state.admit() {
		state.release()
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	diagnostics.enter(AnalyzeDiagnosticPhaseAssemble)
	binding, bindingFailure, valueFailure, allocationFailure := state.newProgramBinding(source)
	diagnostics.Binding = bindingFailure
	diagnostics.ValueSeal = valueFailure
	diagnostics.AllocationCatalog = allocationFailure
	diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageBinding
	if bindingFailure != ProgramBindingFailureNone || binding == nil || binding.binding == nil || !binding.binding.Sealed() {
		state.release()
		diagnostics.failCurrentPhase()
		return nil, CompileUnsupported, diagnostics
	}
	state.binding = binding
	state.admitted = true
	plan := &Plan{state: state}
	runtime.SetFinalizer(plan, func(value *Plan) { _ = value.Close() })
	// No runtime Point, candidate, demand, or WTO authority exists yet. The
	// first Solve owns the sole transition from this cold Plan to an immutable
	// shared runtime topology.
	diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageBinding
	diagnostics.enter(AnalyzeDiagnosticPhaseComplete)
	return plan, CompileComplete, diagnostics
}

// Solve executes or reuses the Plan's one ordinary runtime transaction and
// returns only its detached public projection. A Plan may be solved repeatedly
// and concurrently; Engine serializes the first execution and publishes one
// immutable completed State to every later caller.
func (plan *Plan) Solve(ctx context.Context) (*Result, AnalyzeStatus) {
	result, _, status, _ := plan.solveWithPolicy(ctx, engine.SolveDiagnosticOptions{}, nil, true)
	return result, status
}

// SolveWithDiagnostics executes one fresh source transaction and returns its
// detached analysis phase/reason envelope plus optional engine evidence. A
// zero option selection follows the ordinary solver semantics.
func (plan *Plan) SolveWithDiagnostics(ctx context.Context, options engine.SolveDiagnosticOptions) (*Result, AnalyzeStatus, AnalyzeDiagnostics) {
	result, _, status, diagnostics := plan.solveWithPolicy(ctx, options, nil, false)
	return result, status, diagnostics
}

func (plan *Plan) solveWithPolicy(ctx context.Context, options engine.SolveDiagnosticOptions, policy *DiagnosticPolicy, reuseOrdinary bool) (*Result, *DiagnosticReport, AnalyzeStatus, AnalyzeDiagnostics) {
	if ctx == nil {
		return nil, nil, AnalyzeInvalid, AnalyzeDiagnostics{Phase: AnalyzeDiagnosticPhaseSetup, Reason: AnalyzeDiagnosticReasonInvalidPlan}
	}
	state, leased := plan.acquire()
	if !leased {
		return nil, nil, AnalyzeInvalid, AnalyzeDiagnostics{Phase: AnalyzeDiagnosticPhaseSetup, Reason: AnalyzeDiagnosticReasonInvalidPlan}
	}
	defer state.releaseLease()
	if !options.Valid() {
		return nil, nil, AnalyzeInvalid, AnalyzeDiagnostics{Phase: AnalyzeDiagnosticPhaseSetup, Reason: AnalyzeDiagnosticReasonInvalidOptions}
	}
	var diagnostics AnalyzeDiagnostics
	diagnostics.enter(AnalyzeDiagnosticPhaseAssemble)
	binding := state.binding
	if binding == nil {
		diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageBinding
		diagnostics.enter(AnalyzeDiagnosticPhaseSolve)
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	receiptDiagnostic, topologyOK := state.instantiateRuntimeTopology()
	if !topologyOK {
		applyReceiptAssemblyDiagnostic(&diagnostics, receiptDiagnostic)
		diagnostics.failCurrentPhase()
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	graph := state.graph
	if graph == nil || state.queryPlan == nil {
		diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageRuntime
		diagnostics.enter(AnalyzeDiagnosticPhaseSolve)
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.enter(AnalyzeDiagnosticPhaseSolve)
	diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageRuntime
	var solver *engine.Solver
	var queryPlan *artifactQueryPlan
	var diagnosticObservations []artifactDiagnosticObservationReceipt
	var compiled bool
	if reuseOrdinary && policy == nil && options == (engine.SolveDiagnosticOptions{}) {
		solver, diagnosticObservations, compiled = state.ordinaryRuntimeSolver()
		queryPlan = state.queryPlan
	} else {
		solver, queryPlan, diagnosticObservations, diagnostics.ObservationAttach, compiled = state.buildRuntimeSolver(policy)
	}
	if !compiled || solver == nil || queryPlan == nil {
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	stateResult, solveStatus, engineDiagnostics := solver.SolveWithDiagnostics(ctx, options)
	diagnostics.Engine = engineDiagnostics
	if failure := engineDiagnostics.Failure; failure.Available() {
		// Rule-slot capabilities are intentionally opaque at this boundary;
		// domain diagnostics are classified while artifact rows are attached.
		diagnostics.Rule = AnalyzeDiagnosticRuleUnknown
	}
	diagnostics.ReceiptStage = AnalyzeDiagnosticReceiptStageSolve
	switch solveStatus {
	case engine.SolveCanceled:
		diagnostics.fail(AnalyzeDiagnosticReasonEngineCanceled)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolvePanicked:
		diagnostics.fail(AnalyzeDiagnosticReasonEnginePanicked)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveIncomplete:
		if engineDiagnostics.WorkCutoff {
			diagnostics.fail(AnalyzeDiagnosticReasonWorkCutoff)
		} else {
			diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		}
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveInvalid:
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	case engine.SolveComplete:
		if stateResult == nil {
			diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
			return nil, nil, AnalyzeIncomplete, diagnostics
		}
	default:
		diagnostics.fail(AnalyzeDiagnosticReasonEngineIncomplete)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.enter(AnalyzeDiagnosticPhaseObservation)
	diagnostics.enter(AnalyzeDiagnosticPhaseDetach)
	projection, detached := detachArtifactResult(state.resultReceipt, binding.value.Schema(), policy, queryPlan, diagnosticObservations, graph, solver, stateResult, artifactResultProjectionReceipts{})
	if !detached || projection == nil || projection.result == nil {
		diagnostics.fail(AnalyzeDiagnosticReasonDetach)
		return nil, nil, AnalyzeIncomplete, diagnostics
	}
	diagnostics.enter(AnalyzeDiagnosticPhaseComplete)
	return projection.result, projection.report, AnalyzeComplete, diagnostics
}

// ordinaryRuntimeSolver owns the single receipt compilation used by ordinary
// Plan.Solve calls. Construction is independent of the caller context, so a
// canceled first call cannot poison the immutable compiler or prevent a later
// caller from completing it. Solver itself serializes execution and publishes
// exactly one completed State per accepted runtime revision.
func (state *compiledState) ordinaryRuntimeSolver() (*engine.Solver, []artifactDiagnosticObservationReceipt, bool) {
	if state == nil {
		return nil, nil, false
	}
	state.ordinaryOnce.Do(func() {
		var queryPlan *artifactQueryPlan
		var failure engine.ReceiptObservationAttachFailure
		state.ordinary, queryPlan, state.ordinaryObservations, failure, state.ordinaryOK = state.buildRuntimeSolver(nil)
		state.ordinaryOK = state.ordinaryOK && queryPlan == state.queryPlan && failure == engine.ReceiptObservationAttachFailureNone
		if !state.ordinaryOK {
			state.ordinary = nil
			state.ordinaryObservations = nil
		}
	})
	return state.ordinary, state.ordinaryObservations, state.ordinaryOK
}

// buildRuntimeSolver is the sole runtime binding path. Ordinary solves retain
// its result through ordinaryRuntimeSolver; diagnostic-policy solves invoke it
// afresh because their observation inventory is explicitly flag-controlled.
func (state *compiledState) buildRuntimeSolver(policy *DiagnosticPolicy) (*engine.Solver, *artifactQueryPlan, []artifactDiagnosticObservationReceipt, engine.ReceiptObservationAttachFailure, bool) {
	if state == nil || state.binding == nil || state.binding.binding == nil || state.graph == nil || state.queryPlan == nil || state.artifacts == nil || state.resultReceipt == nil {
		return nil, nil, nil, engine.ReceiptObservationAttachFailureNone, false
	}
	binding, graph, queryPlan := state.binding, state.graph, state.queryPlan
	compilation, compiled := engine.BeginReceiptTopologyCompilation(binding.binding, graph)
	if !compiled || compilation == nil {
		return nil, nil, nil, engine.ReceiptObservationAttachFailureNone, false
	}
	valueIDs, heapIDs, _, witnessOK := linkBootstrapWitness(state, binding)
	compiled = witnessOK && binding.attachLinkBootstrapMembers(compilation, graph, valueIDs, heapIDs) && binding.attachArtifactRuleMembers(compilation, graph, state.artifacts.mounts) && queryPlan.Attach(compilation, graph, binding)
	var observations []artifactDiagnosticObservationReceipt
	var observationFailure engine.ReceiptObservationAttachFailure
	if compiled {
		observations, observationFailure, compiled = attachBranchValueObservations(compilation, graph, binding, state.resultReceipt)
	}
	if !compiled {
		return nil, nil, nil, observationFailure, false
	}
	solver, solved := compilation.Solver()
	if !solved || solver == nil {
		return nil, nil, nil, observationFailure, false
	}
	return solver, queryPlan, observations, observationFailure, true
}

// SourceID is the content fence of the Link compiled into this plan.
func (plan *Plan) SourceID() keyspace.ContentID {
	state, leased := plan.acquire()
	if !leased {
		return keyspace.ContentID{}
	}
	defer state.releaseLease()
	return state.sourceID
}

func (plan *Plan) valid() bool {
	state, leased := plan.acquire()
	if !leased {
		return false
	}
	state.releaseLease()
	return true
}

// Close releases this Plan's assembled topology and domain receipts. Successful
// immutable Program artifacts remain in the content-addressed cache: closing a
// Plan must not force a later equivalent Link to recompile or lower them.
// It is terminal; the finalizer is only a leak safety net.
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
	if state.closing || state.closed || !state.admitted || state.artifacts == nil || !state.resultReceipt.valid() || !state.receipt.Available() ||
		state.binding == nil || state.binding.binding == nil || !state.binding.binding.Sealed() || !state.sourceID.Available() {
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
		state.graph = nil
		state.queryPlan = nil
		state.ordinary = nil
		state.ordinaryObservations = nil
		state.resultReceipt = nil
		state.binding = nil
		state.admitted = false
	})
}

func (state *compiledState) admit() bool {
	if state == nil || state.artifacts == nil || !state.resultReceipt.valid() || !state.receipt.Available() || !state.sourceID.Available() {
		return false
	}
	if len(state.artifacts.mounts) == 0 || len(state.artifacts.byProgram) == 0 {
		return false
	}
	for _, mount := range state.artifacts.mounts {
		if !mount.valid() || mount.artifact == nil || !mount.artifact.Available() {
			return false
		}
	}
	return true
}

func Analyze(ctx context.Context, source *link.Link) (*Result, AnalyzeStatus) {
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

func (result *Result) ContentID() keyspace.ContentID {
	if !result.valid() {
		return keyspace.ContentID{}
	}
	return result.content
}
func (result *Result) SourceID() keyspace.ContentID {
	if !result.valid() {
		return keyspace.ContentID{}
	}
	return result.source
}
func (result *Result) BodyCount() int {
	if !result.valid() {
		return 0
	}
	return len(result.bodies)
}

func (result *Result) BodyAt(index int) (Body, bool) {
	if !result.valid() || index < 0 || index >= len(result.bodies) {
		return Body{}, false
	}
	return Body{owner: result, ordinal: uint32(index + 1)}, true
}

func (body Body) row() (resultBody, bool) {
	if body.owner == nil || !body.owner.valid() || body.ordinal == 0 || uint64(body.ordinal) > uint64(len(body.owner.bodies)) {
		return resultBody{}, false
	}
	return body.owner.bodies[body.ordinal-1], true
}
func (body Body) ID() (keyspace.ContentID, bool) { row, ok := body.row(); return row.id, ok }
func (body Body) RootCount() int {
	// Root rows are the exact mount-qualified ProgramArtifact receipt plane; no
	// Solve-time Program, Source, or Flow reconstruction participates here.
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.roots)
}
func (body Body) RootAt(index int) (Root, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.roots) {
		return Root{}, false
	}
	return Root{owner: body.owner, body: body.ordinal, index: uint32(index + 1)}, true
}
func (root Root) row() (resultRoot, bool) {
	if root.owner == nil || !root.owner.valid() || root.body == 0 || root.index == 0 || uint64(root.body) > uint64(len(root.owner.bodies)) {
		return resultRoot{}, false
	}
	rows := root.owner.bodies[root.body-1].roots
	if uint64(root.index) > uint64(len(rows)) {
		return resultRoot{}, false
	}
	return rows[root.index-1], true
}
func (root Root) ID() (keyspace.ContentID, bool) { row, ok := root.row(); return row.id, ok }
func (root Root) Family() keyspace.Family {
	row, ok := root.row()
	if !ok {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (body Body) EffectDisposition() (present, top, ok bool) {
	row, ok := body.row()
	return row.effectPresent, row.effectTop, ok
}
func (body Body) EffectCount() int {
	row, ok := body.row()
	if !ok {
		return 0
	}
	return len(row.effects)
}
func (body Body) EffectAt(index int) (keyspace.ContentID, bool) {
	row, ok := body.row()
	if !ok || index < 0 || index >= len(row.effects) {
		return keyspace.ContentID{}, false
	}
	return row.effects[index], true
}

// ValueCount and ValueAt expose the per-body projection of the declared Value
// query. A body with no canonical coordinates has a valid empty projection.
func (body Body) ValueCount() int {
	if _, ok := body.row(); !ok || body.owner == nil {
		return 0
	}
	return len(body.owner.values)
}
func (body Body) ValueAt(index int) (id keyspace.ContentID, present, ok bool) {
	row, rowOK := body.row()
	if !rowOK || body.owner == nil || index < 0 || index >= len(body.owner.values) {
		return keyspace.ContentID{}, false, false
	}
	return body.owner.values[index], resultValuePresent(row.valuePresence, index), true
}

func (result *Result) valid() bool {
	// The detached projection is validated and content-addressed exactly once
	// before publication. All fields and nested slices are private and every
	// public accessor is read-only, so replaying the complete body/value/effect
	// census here would be a second authority and makes iteration quadratic.
	return result != nil && result.sealed && result.source.Available() && result.content.Available() && len(result.bodies) != 0
}

func (result *Result) validPayload() bool {
	if result == nil || result.sealed || !result.source.Available() || !result.content.Available() || len(result.bodies) == 0 {
		return false
	}
	for _, value := range result.values {
		if !value.Available() {
			return false
		}
	}
	for _, body := range result.bodies {
		if !body.id.Available() || body.effectTop && len(body.effects) != 0 {
			return false
		}
		for _, root := range body.roots {
			if !root.id.Available() || root.family == keyspace.FamilyInvalid {
				return false
			}
		}
		if !resultValuePresenceValid(body.valuePresence, len(result.values)) {
			return false
		}
		for _, effect := range body.effects {
			if !effect.Available() {
				return false
			}
		}
	}
	if result.placement != nil && !result.placement.valid() {
		return false
	}
	if result.native == nil || !result.native.valid() {
		return false
	}
	return true
}

func analysisResultID(source keyspace.ContentID, values []keyspace.ContentID, bodies []resultBody) (keyspace.ContentID, bool) {
	return analysisResultIDWithProjections(source, values, bodies, nil)
}

func analysisResultIDWithProjections(source keyspace.ContentID, values []keyspace.ContentID, bodies []resultBody, placement *placementResultReceipt) (keyspace.ContentID, bool) {
	return analysisResultIDWithPublication(source, values, bodies, nil, placement)
}

func analysisResultIDWithPublication(source keyspace.ContentID, values []keyspace.ContentID, bodies []resultBody, native *nativePublicationReceipt, placement *placementResultReceipt) (keyspace.ContentID, bool) {
	if !source.Available() || len(bodies) == 0 {
		return keyspace.ContentID{}, false
	}
	hash := sha256.New()
	write := func(value []byte) bool { return writeFramedHash(hash, value) }
	var version, count [8]byte
	binary.BigEndian.PutUint64(version[:], resultFormat)
	binary.BigEndian.PutUint64(count[:], uint64(len(values)))
	if !write([]byte("analysis/result")) || !write(version[:]) || !write(source[:]) || !write(count[:]) {
		return keyspace.ContentID{}, false
	}
	for _, value := range values {
		if !value.Available() || !write(value[:]) {
			return keyspace.ContentID{}, false
		}
	}
	binary.BigEndian.PutUint64(count[:], uint64(len(bodies)))
	if !write(count[:]) {
		return keyspace.ContentID{}, false
	}
	for _, body := range bodies {
		binary.BigEndian.PutUint64(count[:], uint64(len(body.roots)))
		if !write(body.id[:]) || !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, root := range body.roots {
			if !write(root.id[:]) || !write([]byte{byte(root.family)}) {
				return keyspace.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.valuePresence)))
		if !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, word := range body.valuePresence {
			binary.BigEndian.PutUint64(count[:], word)
			if !write(count[:]) {
				return keyspace.ContentID{}, false
			}
		}
		binary.BigEndian.PutUint64(count[:], uint64(len(body.effects)))
		if !write([]byte{boolByte(body.effectPresent), boolByte(body.effectTop)}) || !write(count[:]) {
			return keyspace.ContentID{}, false
		}
		for _, effect := range body.effects {
			if !write(effect[:]) {
				return keyspace.ContentID{}, false
			}
		}
	}
	nativeAvailable := native != nil && native.valid()
	if native != nil && !nativeAvailable {
		return keyspace.ContentID{}, false
	}
	if !write([]byte{boolByte(nativeAvailable)}) {
		return keyspace.ContentID{}, false
	}
	nativeCount := 0
	if nativeAvailable {
		nativeCount = len(native.rows)
	}
	binary.BigEndian.PutUint64(count[:], uint64(nativeCount))
	if !write(count[:]) || nativeAvailable && !write(native.content[:]) {
		return keyspace.ContentID{}, false
	}
	placementAvailable := placement != nil && placement.valid()
	if placement != nil && !placementAvailable {
		return keyspace.ContentID{}, false
	}
	if !write([]byte{boolByte(placementAvailable)}) {
		return keyspace.ContentID{}, false
	}
	// The marker is unavailable-only today. Retain the count field in the
	// Result format so a future solved typed placement receipt extends this
	// identity without a parallel Result family.
	binary.BigEndian.PutUint64(count[:], 0)
	if !write(count[:]) {
		return keyspace.ContentID{}, false
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func resultValueWordCount(values int) int {
	if values <= 0 {
		return 0
	}
	return (values + 63) / 64
}

func resultValuePresent(words []uint64, index int) bool {
	if index < 0 || index/64 >= len(words) {
		return false
	}
	return words[index/64]&(uint64(1)<<uint(index%64)) != 0
}

func setResultValuePresent(words []uint64, index int) bool {
	if index < 0 || index/64 >= len(words) {
		return false
	}
	words[index/64] |= uint64(1) << uint(index%64)
	return true
}

func resultValuePresenceValid(words []uint64, values int) bool {
	if len(words) != resultValueWordCount(values) {
		return false
	}
	if values == 0 || values%64 == 0 {
		return true
	}
	validBits := uint(values % 64)
	return words[len(words)-1]&^((uint64(1)<<validBits)-1) == 0
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
