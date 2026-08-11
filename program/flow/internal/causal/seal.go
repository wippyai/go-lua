package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/recurrence"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Seal materializes the final causal-successor authority from the complete
// typed Flow proofs. The unique root Body in the Body/containment proofs is
// the activation entry; no caller-supplied compatibility entry or source-order
// rescan participates in causal assembly.
func Seal(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	outcomes *outcome.Result,
	control *sourcecontrol.Result,
	recurrenceResult *recurrence.Result,
	ports *evaluation.Ports,
	executableResult *executable.Result,
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
) (*Result, error) {
	state, err := newSealState(sourceView, flow, bodies, forest, outcomes, control, recurrenceResult, ports, executableResult, staticID, moduleID)
	if err != nil {
		return nil, err
	}
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
	if err := state.index.finish(); err != nil {
		return nil, err
	}
	if err := state.pub.result.buildSites(sourceView, control, outcomes); err != nil {
		return nil, err
	}
	return state.pub.result, nil
}

// resultScratch is the one mutable publication target shared by the private
// seal phases. It is a pointer capability, not another row store.
type resultScratch struct{ result *Result }

// typedScratch contains only the seal-local parent projections used by
// structural and tail-route validation.
type typedScratch struct {
	valueParent        []keyspace.Term
	bodyParentRoot     []keyspace.Term
	bodyParentCursor   []int
	invalidExactKeys   []bool
	invalidExactFields []bool
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
	staticID keyspace.ContentID
	moduleID keyspace.ContentID

	counts [keyspace.FamilyCount]uint32
	entry  keyspace.Term
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

type edgeRowsScratch struct {
	edgeRows         []edgeRow
	edgeOwners       []keyspace.Term
	writeCommitEdges []uint32
	writeCommitSet   []bool
}

type boundaryRowsScratch struct {
	boundaryRows   []boundaryRow
	boundaryOwners []keyspace.Term
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

	// Direct Call structural coordinates are filled while the Body root pass
	// visits each root. Boundary sealing performs O(1) lookup rather than
	// rescanning all Body roots for every Call.
	directCallOwner  []keyspace.Term
	directCallCursor []int
	directCallRaw    []keyspace.Term
	directCallSet    []bool
}

type resetState struct {
	*proofState
	arc *arcState
	*edgeRowsScratch
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
}

type indexState struct {
	*proofState
	*arcState
	*edgeRowsScratch
	*boundaryRowsScratch
	*resultScratch
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
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
) (*sealState, error) {
	if bodies == nil || forest == nil || outs == nil || control == nil || recur == nil || ports == nil || exec == nil {
		return nil, errors.New("program/flow/causal: one or more typed prerequisites are unavailable")
	}
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() || sourceView.Identity().TermCount() == 0 {
		return nil, errors.New("program/flow/causal: Source, authored Flow, Static, or Module identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) || !containment.Matches(forest, sourceID, flowID, staticID, moduleID) ||
		!outcome.Matches(outs, sourceID, flowID, staticID, moduleID) || !sourcecontrol.Matches(control, sourceID, flowID, staticID, moduleID) ||
		!recurrence.Matches(recur, sourceID, flowID, staticID, moduleID) || !evaluation.Matches(ports, sourceID, flowID, staticID, moduleID) ||
		!executable.Matches(exec, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/causal: typed prerequisite provenance disagrees with Source, Flow, Static, or Module")
	}

	pub := &resultScratch{result: &Result{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}}
	proof := &proofState{
		source: sourceView, flow: flow, bodies: bodies, forest: forest, outs: outs,
		graph: control, recur: recur, ports: ports, exec: exec,
		staticID: staticID, moduleID: moduleID,
		typedScratch: &typedScratch{}, resultScratch: pub,
	}
	arc := &arcState{proofState: proof}
	edges := &edgeRowsScratch{}
	calls := &callState{proofState: proof}
	reset := &resetState{proofState: proof, arc: arc, edgeRowsScratch: edges}
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
	if control.ArcCount() != recur.ArcCount() {
		return nil, errors.New("program/flow/causal: recurrence Arc denominator disagrees with sourcecontrol")
	}
	arc.arcDisposition = make([]arcDisposition, control.ArcCount())
	if err := proof.buildTypedIndexes(); err != nil {
		return nil, err
	}
	pub.result.index.familyCounts = proof.counts
	calls.callPlans = make([]callNormalRoute, proof.counts[keyspace.FamilyCall]+1)
	calls.callPlanSet = make([]bool, proof.counts[keyspace.FamilyCall]+1)
	calls.tailPlans = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallOwner = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallCursor = make([]int, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallRaw = make([]keyspace.Term, proof.counts[keyspace.FamilyCall]+1)
	calls.directCallSet = make([]bool, proof.counts[keyspace.FamilyCall]+1)
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
