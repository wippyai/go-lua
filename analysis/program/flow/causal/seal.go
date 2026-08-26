package causal

import (
	"errors"
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/recurrence"
	"github.com/wippyai/go-lua/analysis/program/flow/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/runtimeentry"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Preparation is the one-shot causal route transaction. It contains final
// row scratch plus the sealed neutral plan needed by recurrence while its SCC
// partition is live. It is not a published causal authority.
type Preparation struct {
	shared *preparationState
}

type preparationState struct {
	mu            sync.Mutex
	state         *sealState
	plan          *routeplan.Plan
	outcomePhases *sourcecontrol.OutcomePhases
	used          bool
}

// installStructuralPaths installs the parent-issued semantic path view used
// by final route rows. Paths are parallel to the Source family denominators;
// zero entries are rejected when a route later requires that role.
func (r *Result) installStructuralPaths(paths *semanticpath.CausalPaths) bool {
	if r == nil || paths == nil || r.structuralPaths != nil || !paths.Matches(r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return false
	}
	// The certificate already checked the exact Source family denominator.
	// Causal's denominator must agree before it retains the opaque projection.
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if int(r.index.familyCounts[family])+1 == 0 {
			return false
		}
	}
	r.structuralPaths = paths
	return true
}

// PrepareRoutePlanWithStructuralPaths is Flow's production route transaction.
// The structural term path plane was sealed immediately after Source commit;
// Causal installs its immutable view before it emits or binds any final route.
func PrepareRoutePlanWithStructuralPaths(
	sourceView source.View, flow authored.View, bodies *body.Result, forest *containment.Result,
	outcomes *outcome.Result, control *sourcecontrol.Result, ports *evaluation.Ports,
	executableResult *executable.Result, entries *runtimeentry.Result, paths *semanticpath.CausalPaths, outcomePhases *sourcecontrol.OutcomePhases,
	staticID identity.ContentID, moduleID identity.ContentID,
) (*Preparation, error) {
	if paths == nil || outcomePhases == nil || !outcomePhases.Matches(control) {
		return nil, errors.New("program/flow/causal: parent path view is required")
	}
	return prepareRoutePlan(sourceView, flow, bodies, forest, outcomes, control, ports, executableResult, entries, paths, outcomePhases, staticID, moduleID)
}

func prepareRoutePlan(
	sourceView source.View, flow authored.View, bodies *body.Result, forest *containment.Result,
	outcomes *outcome.Result, control *sourcecontrol.Result, ports *evaluation.Ports,
	executableResult *executable.Result, entries *runtimeentry.Result, paths *semanticpath.CausalPaths, outcomePhases *sourcecontrol.OutcomePhases,
	staticID identity.ContentID, moduleID identity.ContentID,
) (*Preparation, error) {
	state, err := newSealState(sourceView, flow, bodies, forest, outcomes, control, nil, ports, executableResult, entries, staticID, moduleID)
	if err != nil {
		return nil, err
	}
	if paths == nil || !state.pub.result.installStructuralPaths(paths) {
		return nil, errors.New("program/flow/causal: parent structural path view is malformed")
	}
	if err := state.pub.result.captureOutcomePhasePaths(control, outcomes); err != nil {
		return nil, err
	}
	builder, err := routeplan.New(control.Owner())
	if err != nil {
		return nil, err
	}
	state.plan = &planState{builder: builder, arcOrdinal: make([]int, control.ArcCount())}
	for index := range state.plan.arcOrdinal {
		state.plan.arcOrdinal[index] = -1
	}
	state.reset.planState = state.plan
	state.boundary.planState = state.plan
	if err := state.eval.emitEvaluation(); err != nil {
		return nil, err
	}
	if err := state.structure.emitStructure(); err != nil {
		return nil, err
	}
	if err := state.outcomes.emitOutcomes(); err != nil {
		return nil, err
	}
	if err := state.boundary.emitBoundaries(); err != nil {
		return nil, err
	}
	plan, err := builder.Seal()
	if err != nil {
		return nil, err
	}
	state.plan.plan = plan
	return &Preparation{shared: &preparationState{state: state, plan: plan, outcomePhases: outcomePhases}}, nil
}

// Seal runs the only legal consumer sequence for a prepared route plan:
// recurrence binds it once while SCC parts are live, then Causal consumes that
// exact binding. Plan and Binding never leave this one-shot transaction.
func (preparation *Preparation) Seal() (*Result, error) {
	if preparation == nil || preparation.shared == nil {
		return nil, errors.New("program/flow/causal: route preparation is unavailable")
	}
	shared := preparation.shared
	shared.mu.Lock()
	if shared.state == nil || shared.plan == nil || shared.outcomePhases == nil || shared.used {
		shared.mu.Unlock()
		return nil, errors.New("program/flow/causal: route preparation is already terminal")
	}
	// Sealing is terminal even on a malformed plan. This prevents a second
	// recurrence issuance for the same Causal transaction.
	state, plan, outcomePhases := shared.state, shared.plan, shared.outcomePhases
	shared.used = true
	shared.plan = nil
	shared.state = nil
	shared.outcomePhases = nil
	shared.mu.Unlock()
	// Every terminal path drops the seal-local scratch. No partially decorated
	// Result is published unless finalizePrepared returns successfully.
	defer state.releaseTransaction()
	recur, binding, err := recurrence.SealWithPlan(state.proof.source, state.proof.flow, state.proof.bodies, state.proof.forest, state.proof.graph, plan, outcomePhases, state.proof.staticID, state.proof.moduleID)
	if err != nil {
		return nil, err
	}
	defer binding.Abort(plan)
	result, err := finalizePrepared(state, plan, recur, binding)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// finalizePrepared is deliberately private: only Preparation.Seal obtains the
// matching Plan and Binding. Keeping this sequence inside Causal prevents an
// honest caller from replaying one Plan through recurrence a second time.
func finalizePrepared(state *sealState, plan *routeplan.Plan, recur *recurrence.Result, binding *recurrence.Binding) (*Result, error) {
	if state == nil || plan == nil || recur == nil || binding == nil {
		return nil, errors.New("program/flow/causal: route preparation or recurrence binding is unavailable")
	}
	if !recurrence.Matches(recur, state.proof.result.sourceID, state.proof.result.flowID, state.proof.result.staticID, state.proof.result.moduleID) ||
		recur.ArcCount() != state.proof.graph.ArcCount() {
		return nil, errors.New("program/flow/causal: recurrence binding provenance disagrees with route preparation")
	}
	if !binding.Matches(plan, recur) {
		return nil, errors.New("program/flow/causal: recurrence binding belongs to another seal result or route plan")
	}
	if err := state.index.validateArcCoverage(); err != nil {
		return nil, err
	}
	state.proof.recur = recur
	if err := state.finalizeBinding(plan, binding); err != nil {
		return nil, err
	}
	// The nested hierarchy is a second one-shot recurrence certificate. It is
	// deliberately consumed before the component directory closes; WTO
	// publication waits until Flow has issued every structural path.
	hierarchy, ok := binding.CompleteAndTakeHierarchy(plan)
	if !ok || hierarchy.Count() == 0 {
		return nil, errors.New("program/flow/causal: nested hierarchy binding was not consumed exactly once")
	}
	state.pub.result.pendingWTO = hierarchy
	components, ok := binding.CompleteAndTakeDirectory(plan)
	if !ok {
		return nil, errors.New("program/flow/causal: recurrence binding was not consumed exactly once")
	}
	if err := state.installComponentDirectory(components); err != nil {
		return nil, err
	}
	if err := state.index.finish(); err != nil {
		return nil, err
	}
	if err := state.pub.result.installBoundRoutePaths(); err != nil {
		return nil, fmt.Errorf("program/flow/causal: %w", err)
	}
	if err := state.pub.result.buildSites(state.proof.source, state.proof.graph, state.proof.outs); err != nil {
		return nil, err
	}
	if err := state.pub.result.prepareWTORows(); err != nil {
		return nil, err
	}
	if !state.pub.result.installRouteSemanticPaths() {
		return nil, errors.New("program/flow/causal: final route publication phase=semantic-paths row=-1 reason=path-or-directory-mismatch")
	}
	if err := state.pub.result.finalizeLocalWTO(); err != nil {
		return nil, err
	}
	return state.pub.result, nil
}

func (s *sealState) releaseTransaction() {
	if s == nil {
		return
	}
	if s.plan != nil {
		s.plan.builder = nil
		s.plan.plan = nil
		s.plan.arcOrdinal = nil
	}
	s.plan = nil
	if s.edges != nil {
		s.edges.edgeRows, s.edges.edgeOwners, s.edges.planOrdinals = nil, nil, nil
	}
	if s.rows != nil {
		s.rows.boundaryRows, s.rows.boundaryOwners, s.rows.planOrdinals = nil, nil, nil
	}
	if s.arc != nil {
		s.arc.arcDisposition = nil
	}
	// Proof inputs are no longer needed after indexes and Sites have been
	// built. Result contains only its final typed rows/projections.
	if s.proof != nil {
		s.proof.graph, s.proof.recur, s.proof.ports, s.proof.exec = nil, nil, nil, nil
		s.proof.bodies, s.proof.forest, s.proof.outs = nil, nil, nil
		s.proof.typedScratch = nil
	}
}

// resultScratch is the one mutable publication target shared by the private
// seal phases. It is a pointer capability, not another row store.
type resultScratch struct{ result *Result }

// typedScratch contains only the seal-local parent projections used by
// structural and tail-route validation.
type typedScratch struct {
	valueParent          []keyspace.Term
	bodyParentRoot       []keyspace.Term
	bodyParentCursor     []int
	invalidExactKeys     []bool
	invalidExactFields   []bool
	tableFieldThrowProof []sourcecontrol.TableFieldThrowEligibility
}

// proofState owns immutable typed prerequisites and their denominator fence.
// It is the only child allowed to answer prerequisite/liveness questions.
type proofState struct {
	source   source.View
	flow     authored.View
	bodies   *body.Result
	forest   *containment.Result
	outs     *outcome.Result
	graph    *sourcecontrol.Result
	recur    *recurrence.Result
	ports    *evaluation.Ports
	exec     *executable.Result
	entries  *runtimeentry.Result
	staticID identity.ContentID
	moduleID identity.ContentID

	counts [keyspace.FamilyCount]uint32
	entry  keyspace.Term
	// unreachedRepeatControls are the Repeat conditions whose child Body tail
	// is unreachable. They remain executable subjects and never evaluate.
	unreachedRepeatControls []keyspace.Term
	*typedScratch
	*resultScratch
}

type arcState struct {
	*proofState
	// arcDisposition is a seal-only exact ledger. Every sourcecontrol Arc is
	// claimed once as a local route, a Call normal boundary, an explicit
	// liveness/function-availability disposition, or dead/static.
	arcDisposition []arcDisposition
}

// planState is seal-local glue between final Causal rows and the recurrence
// binding ordinal. It contains neither adjacency nor SCC data.
type planState struct {
	builder     *routeplan.Builder
	plan        *routeplan.Plan
	arcOrdinal  []int
	nextOrdinal int
}

type edgeRowsScratch struct {
	edgeRows         []edgeRow
	edgeOwners       []keyspace.Term
	planOrdinals     []int
	writeCommitEdges []uint32
	writeCommitSet   []bool
}

type boundaryRowsScratch struct {
	boundaryRows   []boundaryRow
	boundaryOwners []keyspace.Term
	planOrdinals   []boundaryPlanOrdinals
}

type boundaryPlanOrdinals struct {
	ordinals [BoundaryCancel + 1]int
	present  [BoundaryCancel + 1]bool
}

type callState struct {
	*proofState
	// callPlans are one dense seal-local normal-disposition authority. Typed
	// evaluation and structural passes fill each live Call exactly once; the
	// published boundary rows copy the selected value and then discard these
	// arrays.
	callPlans   []callNormalRoute
	callPlanSet []bool
	tailPlans   []keyspace.Term
	tailProofs  []sourcecontrol.CallTailProof

	// Direct Call structural coordinates are filled while the Body root pass
	// visits each root. Boundary sealing performs O(1) lookup rather than
	// rescanning all Body roots for every Call.
	directCallOwner  []keyspace.Term
	directCallCursor []int
	directCallRaw    []keyspace.Term
	directCallSet    []bool
	normalArc        []int
}

type resetState struct {
	*proofState
	arc *arcState
	*edgeRowsScratch
	*planState
	tailProofs []sourcecontrol.CallTailProof
}

type evalState struct {
	*proofState
	*arcState
	*resetState
	*edgeRowsScratch
	*callState
}

// routeState is the shared typed structural-destination capability. Keeping
// it separate leaves evaluation materialization bounded while allowing both
// structural and outcome phases to use the same Entry/Resume conversion.
type routeState struct{ *evalState }
type structureState struct{ *routeState }
type outcomeState struct{ *routeState }

type boundaryState struct {
	*structureState
	*callState
	*boundaryRowsScratch
	*planState
}

type indexState struct {
	*proofState
	*arcState
	*edgeRowsScratch
	*boundaryRowsScratch
	*resultScratch
	arcCoverageValidated bool
}

// sealState is only the phase coordinator. Relation-owned scratch and all
// materialization methods live on the narrow child components above.
type sealState struct {
	proof     *proofState
	arc       *arcState
	reset     *resetState
	eval      *evalState
	structure *structureState
	outcomes  *outcomeState
	boundary  *boundaryState
	index     *indexState
	calls     *callState
	edges     *edgeRowsScratch
	rows      *boundaryRowsScratch
	pub       *resultScratch
	plan      *planState
}

func newSealState(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	outs *outcome.Result,
	control *sourcecontrol.Result,
	recur *recurrence.Result,
	ports *evaluation.Ports,
	exec *executable.Result,
	entries *runtimeentry.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*sealState, error) {
	if bodies == nil || forest == nil || outs == nil || control == nil || ports == nil || exec == nil || entries == nil {
		return nil, errors.New("program/flow/causal: one or more typed prerequisites are unavailable")
	}
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() || sourceView.Identity().TermCount() == 0 {
		return nil, errors.New("program/flow/causal: Source, authored Flow, Static, or Module identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) || !containment.Matches(forest, sourceID, flowID, staticID, moduleID) ||
		!outcome.Matches(outs, sourceID, flowID, staticID, moduleID) || !sourcecontrol.Matches(control, sourceID, flowID, staticID, moduleID) ||
		(recur != nil && !recurrence.Matches(recur, sourceID, flowID, staticID, moduleID)) || !evaluation.Matches(ports, sourceID, flowID, staticID, moduleID) ||
		!executable.Matches(exec, sourceID, flowID, staticID, moduleID) || !runtimeentry.Matches(entries, sourceID, flowID, staticID, moduleID) ||
		!runtimeentry.OwnsParents(entries, control, ports, exec) {
		return nil, errors.New("program/flow/causal: typed prerequisite provenance disagrees with Source, Flow, Static, or Module")
	}

	pub := &resultScratch{result: &Result{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}}
	proof := &proofState{
		source: sourceView, flow: flow, bodies: bodies, forest: forest, outs: outs,
		graph: control, recur: recur, ports: ports, exec: exec, entries: entries,
		staticID: staticID, moduleID: moduleID,
		typedScratch: &typedScratch{}, resultScratch: pub,
	}
	arc := &arcState{proofState: proof}
	edges := &edgeRowsScratch{}
	calls := &callState{proofState: proof}
	reset := &resetState{proofState: proof, arc: arc, edgeRowsScratch: edges, tailProofs: calls.tailProofs}
	eval := &evalState{proofState: proof, arcState: arc, resetState: reset, edgeRowsScratch: edges, callState: calls}
	routes := &routeState{evalState: eval}
	structure := &structureState{routeState: routes}
	outcomes := &outcomeState{routeState: routes}
	rows := &boundaryRowsScratch{}
	boundary := &boundaryState{structureState: structure, callState: calls, boundaryRowsScratch: rows}
	index := &indexState{proofState: proof, arcState: arc, edgeRowsScratch: edges, boundaryRowsScratch: rows, resultScratch: pub}
	state := &sealState{proof: proof, arc: arc, reset: reset, eval: eval, structure: structure,
		outcomes: outcomes, boundary: boundary, index: index, calls: calls, edges: edges, rows: rows, pub: pub}
	if err := proof.loadCounts(); err != nil {
		return nil, err
	}
	for ordinal := uint32(1); ordinal <= proof.counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		_, loopBody, loopKind, loopControl, loopOK := flow.Control().Loops().Get(loop)
		if !loopOK || loopKind != kind.LoopRepeat || loopControl == 0 {
			continue
		}
		tail, tailOK := control.Tail(loopBody)
		if !tailOK {
			return nil, errors.New("program/flow/causal: Repeat child tail is unavailable")
		}
		if !control.Reachable(tail) {
			proof.unreachedRepeatControls = append(proof.unreachedRepeatControls, loopControl)
		}
	}
	edges.writeCommitEdges = make([]uint32, proof.counts[keyspace.FamilyWrite]+1)
	edges.writeCommitSet = make([]bool, proof.counts[keyspace.FamilyWrite]+1)
	for ordinal := uint32(1); ordinal <= proof.counts[keyspace.FamilyBody]; ordinal++ {
		candidate := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		if _, hasParent := bodies.Parent(candidate); !hasParent {
			if proof.entry != 0 {
				return nil, errors.New("program/flow/causal: Body proof has multiple Entry roots")
			}
			proof.entry = candidate
		}
	}
	if !validPreTerm(proof.entry, proof.counts) || keyspace.TermFamily(proof.entry) != keyspace.FamilyBody {
		return nil, errors.New("program/flow/causal: Entry Body is unavailable")
	}
	if _, hasParent := bodies.Parent(proof.entry); hasParent {
		return nil, errors.New("program/flow/causal: Entry Body is not a forest root")
	}
	if _, activationOK := bodies.Activation(proof.entry); !activationOK {
		return nil, errors.New("program/flow/causal: Entry Body activation is unavailable")
	}
	if recur != nil && control.ArcCount() != recur.ArcCount() {
		return nil, errors.New("program/flow/causal: recurrence Arc denominator disagrees with sourcecontrol")
	}
	arc.arcDisposition = make([]arcDisposition, control.ArcCount())
	proof.tableFieldThrowProof = make([]sourcecontrol.TableFieldThrowEligibility, proof.counts[keyspace.FamilyTableField]+1)
	if err := proof.buildTypedIndexes(); err != nil {
		return nil, err
	}
	pub.result.index.familyCounts = proof.counts
	calls.callPlans = make([]callNormalRoute, proof.counts[keyspace.FamilyCall]+1)
	calls.callPlanSet = make([]bool, proof.counts[keyspace.FamilyCall]+1)
	calls.tailPlans = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.tailProofs = make([]sourcecontrol.CallTailProof, proof.counts[keyspace.FamilyCall]+1)
	reset.tailProofs = calls.tailProofs
	calls.directCallOwner = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallCursor = make([]int, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallRaw = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallSet = make([]bool, proof.counts[keyspace.FamilyCall]+1)
	calls.normalArc = make([]int, proof.counts[keyspace.FamilyCall]+1)
	for index := range calls.normalArc {
		calls.normalArc[index] = -1
	}
	for index := range calls.directCallCursor {
		calls.directCallCursor[index] = -1
	}
	if err := boundary.buildTailPlans(); err != nil {
		return nil, err
	}
	for _, family := range [...]keyspace.Family{keyspace.FamilyLabel, keyspace.FamilyLoop} {
		pub.result.reset.headRanges[family] = make([]range32, proof.counts[family]+1)
		for ordinal := range pub.result.reset.headRanges[family] {
			pub.result.reset.headRanges[family][ordinal].start = ^uint32(0)
		}
	}
	for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
		pub.result.reset.decisionHead[family] = make([]keyspace.Term, proof.counts[family]+1)
		pub.result.reset.decisionRank[family] = make([]uint32, proof.counts[family]+1)
	}
	return state, nil
}

func (s *proofState) loadCounts() error {
	identity := s.source.Identity()
	var preOutcome uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return errors.New("program/flow/causal: invalid Source family denominator")
		}
		s.counts[family] = uint32(count)
		if family != keyspace.FamilyOutcome {
			preOutcome += uint64(count)
		}
	}
	if preOutcome == 0 || preOutcome+uint64(s.counts[keyspace.FamilyOutcome]) != uint64(identity.TermCount()) ||
		uint64(s.forest.Count()) != preOutcome || s.bodies.BodyCount() != int(s.counts[keyspace.FamilyBody]) ||
		s.outs.Count() != int(s.counts[keyspace.FamilyOutcome]) {
		return errors.New("program/flow/causal: final Source denominator disagrees with typed proofs")
	}
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyValues, s.flow.Values().Count()},
		{keyspace.FamilyLensExact, s.flow.Access().Exact().Count()},
		{keyspace.FamilyLensKey, s.flow.Access().Dynamic().Count()},
		{keyspace.FamilyCell, s.flow.Storage().Cells().Count()},
		{keyspace.FamilyRead, s.flow.Storage().Reads().Count()},
		{keyspace.FamilyVararg, s.flow.Storage().Varargs().Count()},
		{keyspace.FamilyBind, s.flow.Storage().Binds().Count()},
		{keyspace.FamilyAssign, s.flow.Storage().Assigns().Count()},
		{keyspace.FamilyWrite, s.flow.Storage().Writes().Count()},
		{keyspace.FamilyTable, s.flow.Tables().Count()},
		{keyspace.FamilyTableField, s.flow.Fields().Count()},
		{keyspace.FamilyUnary, s.flow.Operators().Unaries().Count()},
		{keyspace.FamilyBinary, s.flow.Operators().Binaries().Count()},
		{keyspace.FamilySelect, s.flow.Operators().Selects().Count()},
		{keyspace.FamilyFunction, s.flow.Functions().Count()},
		{keyspace.FamilyCall, s.flow.Calls().Count()},
		{keyspace.FamilyReturn, s.flow.Control().Returns().Count()},
		{keyspace.FamilyBreak, s.flow.Control().Breaks().Count()},
		{keyspace.FamilyLabel, s.flow.Control().Labels().Count()},
		{keyspace.FamilyGoto, s.flow.Control().Gotos().Count()},
		{keyspace.FamilyBranch, s.flow.Control().Branches().Count()},
		{keyspace.FamilyLoop, s.flow.Control().Loops().Count()},
		{keyspace.FamilyValueClaim, s.flow.Claims().Count()},
		{keyspace.FamilyTypeValue, s.flow.TypeValues().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || uint32(check.count) != s.counts[check.family] {
			return fmt.Errorf("program/flow/causal: authored %v denominator disagrees with Source", check.family)
		}
	}
	return nil
}
