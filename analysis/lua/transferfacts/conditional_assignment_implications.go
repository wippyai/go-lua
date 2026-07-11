package transferfacts

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// addConditionalAssignmentImplications derives merge-point facts of the form:
// if a later path value proves the same branch condition, then a target assigned
// on that branch edge keeps the value assigned on that edge. This preserves correlations such as
// `if not use_template then executor = make() end` across the join, while still
// requiring every path on the selected edge to end with a compatible write and
// the opposite edge to contradict the trigger value.
func (l *lowerer) addConditionalAssignmentImplications(input *factflow.FactsInput, graph cfg.Graph) {
	if input == nil {
		return
	}
	l.addChannelSelectPayloadImplications(input, graph)
	if graph == nil || len(input.RootAssignments) == 0 {
		return
	}
	postdom := dominance.ComputeImmediatePostDominators(graph)
	if len(postdom) == 0 {
		return
	}
	rpo := graph.RPO()
	continuationJoins := newBranchContinuationJoinCache(graph, rpo)
	incomingByPoint := l.assignmentStatesBeforePoints(input, graph)
	candidates := make([]conditionalAssignmentImplicationCandidate, 0)
	for _, branch := range rpo {
		if !graph.IsBranch(branch) {
			continue
		}
		join, ok := postdom[branch]
		if ok && join == graph.Exit() {
			join, ok = continuationJoins.join(branch)
		}
		if !ok || join == branch || join == graph.Exit() {
			continue
		}
		incoming, ok := incomingByPoint[branch]
		if !ok {
			incoming = l.assignmentStateBeforePoint(input, graph, branch)
		}
		edgeAssignments := make(map[cfg.Point]presentAssignmentState)
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			edgeAssignments[succ] = l.presentAssignmentsOnBranchEdge(input, graph, succ, join, incoming)
		}
		for succ, selected := range edgeAssignments {
			candidates = append(candidates, l.branchAssignmentValueImplicationCandidates(join, incoming, selected, oppositeBranchAssignmentStates(graph, branch, succ, edgeAssignments))...)
		}
		refinements := input.BranchRefinements[branch].Refinements()
		if len(refinements) == 0 {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if !ok {
				continue
			}
			present := edgeAssignments[succ]
			if present.len() == 0 {
				continue
			}
			for _, trigger := range branchEdgeValueTriggers(l.registry, refinements, cond) {
				if !branchTriggerContradictedOnEdge(l.registry, refinements, trigger, !cond) {
					continue
				}
				for _, target := range present.entries {
					implication, ok := conditionalAssignmentTargetImplication(
						conditionalImplicationTrigger{path: trigger.path, value: trigger.value},
						target,
					)
					if ok {
						candidates = append(candidates, conditionalAssignmentImplicationCandidate{point: join, implication: implication})
					}
				}
			}
		}
	}
	for _, candidate := range canonicalConditionalAssignmentImplicationCandidates(l.registry, candidates) {
		appendPathValuePresenceImplications(input.PathValuePresenceImplications, candidate.point, candidate.implication)
	}
}

// assignmentStatesBeforePoints computes the incoming assignment state at every
// branch. A branch outside a cycle can use the ordinary whole-graph solution:
// its own transfer cannot reach any of its predecessors. A branch in a cycle
// needs the historical "first arrival" meaning, where the branch is removed
// before solving so a previous loop iteration cannot feed that same branch.
//
// The cyclic case is solved only inside the branch's strongly-connected
// component. Edges entering that component are seeded from the ordinary global
// solution. This is exact: if an outside predecessor depended on the removed
// branch and could then enter its component, it would be mutually reachable
// with the branch and therefore belong to the same component.
func (l *lowerer) assignmentStatesBeforePoints(
	input *factflow.FactsInput,
	graph cfg.Graph,
) map[cfg.Point]presentAssignmentState {
	if input == nil || graph == nil {
		return nil
	}
	rpo := cfg.RPOReadOnly(graph)
	if len(rpo) == 0 {
		return nil
	}
	in, out := l.solvePresentAssignmentStates(input, graph, nil, -1, noPresentAssignmentStop, nil)
	components, componentOf := presentAssignmentStrongComponents(graph, rpo)
	result := make(map[cfg.Point]presentAssignmentState)
	for _, point := range rpo {
		if !graph.IsBranch(point) {
			continue
		}
		componentIndex, ok := componentOf[point]
		if !ok || !presentAssignmentComponentIsCyclic(graph, components[componentIndex]) {
			if state, exists := in[point]; exists {
				result[point] = state
			}
			continue
		}
		result[point] = l.assignmentStateBeforePointInComponent(
			input,
			graph,
			point,
			componentOf,
			componentIndex,
			out,
		)
	}
	return result
}

const noPresentAssignmentStop = ^cfg.Point(0)

func (l *lowerer) addChannelSelectPayloadImplications(
	input *factflow.FactsInput,
	graph cfg.Graph,
) {
	if l == nil || l.registry == nil || input == nil || graph == nil {
		return
	}
	for _, point := range graph.RPO() {
		for _, event := range input.ChannelSelects[point].Events() {
			if event.Kind() != factflow.ChannelSelectReceive {
				continue
			}
			resultPath, ok := event.ResultPath()
			if !ok || resultPath.IsEmpty() || resultPath.Symbol == 0 {
				continue
			}
			casePath, ok := event.CasePath()
			if !ok || casePath.IsEmpty() || casePath.Symbol == 0 {
				continue
			}
			payload, ok := event.PayloadValue()
			if !ok ||
				product.Equal(l.registry, payload, product.Top()) ||
				product.Equal(l.registry, payload, product.Bottom(l.registry)) {
				continue
			}
			appendPathValuePresenceImplications(
				input.PathValuePresenceImplications,
				point,
				factflow.NewPathEqualityValueRefinementImplication(
					resultPath.Field("channel"),
					casePath,
					resultPath.Field("value"),
					payload,
				),
			)
		}
	}
}

func (l *lowerer) branchAssignmentValueImplicationCandidates(
	join cfg.Point,
	incoming presentAssignmentState,
	selected presentAssignmentState,
	opposites []presentAssignmentState,
) []conditionalAssignmentImplicationCandidate {
	if selected.len() == 0 || len(opposites) == 0 {
		return nil
	}
	var out []conditionalAssignmentImplicationCandidate
	for triggerSym, triggerAssignment := range selected.entries {
		if !triggerAssignment.fromBranch || !triggerAssignment.hasValue {
			continue
		}
		if !literalPartitionDiscriminantValue(l.registry, triggerAssignment.value) {
			continue
		}
		if !oppositeAssignmentsContradictTrigger(l.registry, triggerSym, triggerAssignment, incoming, opposites) {
			continue
		}
		trigger := conditionalImplicationTrigger{
			path:          triggerAssignment.path,
			value:         triggerAssignment.value,
			requireTruthy: true,
		}
		for targetSym, target := range selected.entries {
			if targetSym == triggerSym {
				continue
			}
			implication, ok := conditionalAssignmentTargetImplication(trigger, target)
			if ok {
				out = append(out, conditionalAssignmentImplicationCandidate{point: join, implication: implication})
			}
		}
	}
	return out
}

type conditionalImplicationTrigger struct {
	path          path.Path
	value         product.Value
	requireTruthy bool
}

func conditionalAssignmentTargetImplication(
	trigger conditionalImplicationTrigger,
	target conditionalAssignment,
) (factflow.PathValuePresenceImplication, bool) {
	if target.path.Symbol == 0 || !target.fromBranch {
		return factflow.PathValuePresenceImplication{}, false
	}
	var implication factflow.PathValuePresenceImplication
	switch {
	case target.hasValue && trigger.requireTruthy:
		implication = factflow.NewPathTruthyValueRefinementImplication(trigger.path, trigger.value, target.path, target.value)
	case target.hasValue:
		implication = factflow.NewPathValueRefinementImplication(trigger.path, trigger.value, target.path, target.value)
	default:
		implication = factflow.NewPathValuePresenceImplication(trigger.path, trigger.value, target.path, presence.Present())
	}
	return implication, true
}

// conditionalAssignmentImplicationCandidate is sorted and deduplicated before
// publication. Its key uses only CFG point, canonical path identity, implication
// shape, and stable product hashes; it deliberately excludes pointer identity and
// diagnostic formatting.
type conditionalAssignmentImplicationCandidate struct {
	point       cfg.Point
	implication factflow.PathValuePresenceImplication
}

type conditionalAssignmentImplicationKey struct {
	point               cfg.Point
	triggerPath         path.PathKey
	triggerOtherPath    path.PathKey
	triggerValue        uint64
	triggerPresence     uint8
	hasTriggerPresence  bool
	hasTriggerPathEqual bool
	targetPath          path.PathKey
	targetPresence      uint8
	targetValue         uint64
	hasTargetValue      bool
}

func canonicalConditionalAssignmentImplicationCandidates(reg *axis.Registry, candidates []conditionalAssignmentImplicationCandidate) []conditionalAssignmentImplicationCandidate {
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := conditionalAssignmentImplicationKeyFor(reg, candidates[i]), conditionalAssignmentImplicationKeyFor(reg, candidates[j])
		return left.less(right)
	})
	out := candidates[:0]
	for _, candidate := range candidates {
		if len(out) != 0 && conditionalAssignmentImplicationCandidateEqual(reg, out[len(out)-1], candidate) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func conditionalAssignmentImplicationKeyFor(reg *axis.Registry, candidate conditionalAssignmentImplicationCandidate) conditionalAssignmentImplicationKey {
	implication := candidate.implication
	return conditionalAssignmentImplicationKey{
		point:               candidate.point,
		triggerPath:         implication.TriggerPathRef().Key(),
		triggerOtherPath:    implication.TriggerOtherPathRef().Key(),
		triggerValue:        product.Hash(reg, implication.TriggerValue()),
		triggerPresence:     uint8(implication.TriggerPresence()),
		hasTriggerPresence:  implication.HasTriggerPresence(),
		hasTriggerPathEqual: implication.HasTriggerPathEqual(),
		targetPath:          implication.TargetPathRef().Key(),
		targetPresence:      uint8(implication.TargetPresence()),
		targetValue:         product.Hash(reg, implication.TargetValue()),
		hasTargetValue:      implication.HasTargetValue(),
	}
}

func (key conditionalAssignmentImplicationKey) less(other conditionalAssignmentImplicationKey) bool {
	if key.point != other.point {
		return key.point < other.point
	}
	if key.triggerPath != other.triggerPath {
		return key.triggerPath < other.triggerPath
	}
	if key.triggerOtherPath != other.triggerOtherPath {
		return key.triggerOtherPath < other.triggerOtherPath
	}
	if key.triggerValue != other.triggerValue {
		return key.triggerValue < other.triggerValue
	}
	if key.triggerPresence != other.triggerPresence {
		return key.triggerPresence < other.triggerPresence
	}
	if key.hasTriggerPresence != other.hasTriggerPresence {
		return !key.hasTriggerPresence
	}
	if key.hasTriggerPathEqual != other.hasTriggerPathEqual {
		return !key.hasTriggerPathEqual
	}
	if key.targetPath != other.targetPath {
		return key.targetPath < other.targetPath
	}
	if key.targetPresence != other.targetPresence {
		return key.targetPresence < other.targetPresence
	}
	if key.targetValue != other.targetValue {
		return key.targetValue < other.targetValue
	}
	return !key.hasTargetValue && other.hasTargetValue
}

func conditionalAssignmentImplicationCandidateEqual(reg *axis.Registry, left, right conditionalAssignmentImplicationCandidate) bool {
	if left.point != right.point {
		return false
	}
	leftImplication, rightImplication := left.implication, right.implication
	return leftImplication.TriggerPathRef().Equal(rightImplication.TriggerPathRef()) &&
		leftImplication.TriggerOtherPathRef().Equal(rightImplication.TriggerOtherPathRef()) &&
		product.Equal(reg, leftImplication.TriggerValue(), rightImplication.TriggerValue()) &&
		leftImplication.TriggerPresence() == rightImplication.TriggerPresence() &&
		leftImplication.HasTriggerPresence() == rightImplication.HasTriggerPresence() &&
		leftImplication.HasTriggerPathEqual() == rightImplication.HasTriggerPathEqual() &&
		leftImplication.TargetPathRef().Equal(rightImplication.TargetPathRef()) &&
		leftImplication.TargetPresence() == rightImplication.TargetPresence() &&
		product.Equal(reg, leftImplication.TargetValue(), rightImplication.TargetValue()) &&
		leftImplication.HasTargetValue() == rightImplication.HasTargetValue()
}

type branchContinuationJoinCache struct {
	graph     cfg.Graph
	rpo       []cfg.Point
	rpoIndex  map[cfg.Point]int
	reachable map[cfg.Point]map[cfg.Point]struct{}
}

func newBranchContinuationJoinCache(graph cfg.Graph, rpo []cfg.Point) branchContinuationJoinCache {
	index := make(map[cfg.Point]int, len(rpo))
	for i, point := range rpo {
		index[point] = i
	}
	return branchContinuationJoinCache{
		graph:     graph,
		rpo:       append([]cfg.Point(nil), rpo...),
		rpoIndex:  index,
		reachable: make(map[cfg.Point]map[cfg.Point]struct{}),
	}
}

func (c branchContinuationJoinCache) join(branch cfg.Point) (cfg.Point, bool) {
	graph := c.graph
	if graph == nil || !graph.IsBranch(branch) {
		return 0, false
	}
	successors := cfg.SuccessorsReadOnly(graph, branch)
	if len(successors) < 2 {
		return 0, false
	}
	counts := make(map[cfg.Point]int)
	for _, succ := range successors {
		reachable := c.reachableBeforeExit(succ)
		if len(reachable) == 0 {
			return 0, false
		}
		for point := range reachable {
			counts[point]++
		}
	}
	branchIndex := c.rpoIndex[branch]
	for _, point := range c.rpo {
		if point == graph.Exit() || c.rpoIndex[point] <= branchIndex {
			continue
		}
		if counts[point] == len(successors) {
			return point, true
		}
	}
	return 0, false
}

func (c branchContinuationJoinCache) reachableBeforeExit(start cfg.Point) map[cfg.Point]struct{} {
	if cached, ok := c.reachable[start]; ok {
		return cached
	}
	graph := c.graph
	if graph == nil || start == graph.Exit() {
		return nil
	}
	seen := map[cfg.Point]struct{}{}
	queue := []cfg.Point{start}
	for len(queue) != 0 {
		point := queue[0]
		queue = queue[1:]
		if point == graph.Exit() {
			continue
		}
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if succ != graph.Exit() {
				queue = append(queue, succ)
			}
		}
	}
	c.reachable[start] = seen
	return seen
}

func oppositeBranchAssignmentStates(
	graph cfg.Graph,
	branch cfg.Point,
	selected cfg.Point,
	states map[cfg.Point]presentAssignmentState,
) []presentAssignmentState {
	var out []presentAssignmentState
	for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
		if succ == selected {
			continue
		}
		out = append(out, states[succ])
	}
	return out
}

func oppositeAssignmentsContradictTrigger(
	reg *axis.Registry,
	triggerKey presentAssignmentKey,
	trigger conditionalAssignment,
	incoming presentAssignmentState,
	opposites []presentAssignmentState,
) bool {
	if reg == nil || len(opposites) == 0 {
		return false
	}
	if incomingTriggerContradictsWithoutOppositeBranchWrite(reg, triggerKey, trigger, incoming, opposites) {
		return true
	}
	for _, opposite := range opposites {
		other, ok := opposite.get(triggerKey)
		if !ok || !other.hasValue {
			other, ok = incoming.get(triggerKey)
		}
		if !ok || !other.hasValue {
			return false
		}
		meet := product.Meet(reg, trigger.value, other.value)
		if !product.Equal(reg, meet, product.Bottom(reg)) && !presence.Equal(product.PresenceOf(meet), presence.Bottom()) {
			return false
		}
	}
	return true
}

func incomingTriggerContradictsWithoutOppositeBranchWrite(
	reg *axis.Registry,
	triggerKey presentAssignmentKey,
	trigger conditionalAssignment,
	incoming presentAssignmentState,
	opposites []presentAssignmentState,
) bool {
	baseline, ok := incoming.get(triggerKey)
	if !ok || !baseline.hasValue {
		return false
	}
	for _, opposite := range opposites {
		if other, ok := opposite.get(triggerKey); ok && other.fromBranch {
			return false
		}
	}
	meet := product.Meet(reg, trigger.value, baseline.value)
	return product.Equal(reg, meet, product.Bottom(reg)) || presence.Equal(product.PresenceOf(meet), presence.Bottom())
}

func literalBoolValue(reg *axis.Registry, value product.Value) (bool, bool) {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return false, false
	}
	switch {
	case typ.TypeEquals(t, typ.True):
		return true, true
	case typ.TypeEquals(t, typ.False):
		return false, true
	default:
		return false, false
	}
}

func literalPartitionDiscriminantValue(reg *axis.Registry, value product.Value) bool {
	if _, ok := literalBoolValue(reg, value); ok {
		return true
	}
	_, ok := typevalue.StringLiteralOf(reg, value)
	return ok
}

type branchValueTrigger struct {
	path  path.Path
	value product.Value
}

func branchEdgeValueTriggers(reg *axis.Registry, refinements []factflow.BranchRefinement, cond bool) []branchValueTrigger {
	if reg == nil {
		return nil
	}
	var out []branchValueTrigger
	for _, refinement := range refinements {
		value, ok := refinement.ValueForEdge(cond)
		if !ok {
			continue
		}
		if value.FalsyAbsent() {
			continue
		}
		constraint, ok := value.Constraint()
		if !ok || product.Equal(reg, constraint, product.Bottom(reg)) || constraint == product.Top() {
			continue
		}
		out = append(out, branchValueTrigger{path: refinement.TargetPath(), value: constraint})
	}
	return out
}

func branchTriggerContradictedOnEdge(reg *axis.Registry, refinements []factflow.BranchRefinement, trigger branchValueTrigger, cond bool) bool {
	if reg == nil || trigger.path.IsEmpty() {
		return false
	}
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(trigger.path) {
			continue
		}
		value, ok := refinement.ValueForEdge(cond)
		if !ok {
			continue
		}
		constraint, ok := value.Constraint()
		if !ok {
			continue
		}
		meet := product.Meet(reg, trigger.value, constraint)
		return product.Equal(reg, meet, product.Bottom(reg)) || presence.Equal(product.PresenceOf(meet), presence.Bottom())
	}
	return false
}

type conditionalAssignment struct {
	path       path.Path
	value      product.Value
	hasValue   bool
	fromBranch bool
}

type presentAssignmentKey path.PathKey

// presentAssignmentState is an immutable dataflow value. entries must only be
// mutated by presentAssignmentStateEditor after copy-on-write. identity names
// the exact immutable generation, making the common convergence check O(1).
// Independently constructed but equal states still compare structurally.
type presentAssignmentState struct {
	entries  map[presentAssignmentKey]conditionalAssignment
	identity *presentAssignmentStateIdentity
}

type presentAssignmentStateIdentity struct{ _ byte }

func newPresentAssignmentState(entries map[presentAssignmentKey]conditionalAssignment) presentAssignmentState {
	if len(entries) == 0 {
		return presentAssignmentState{}
	}
	return presentAssignmentState{entries: entries, identity: &presentAssignmentStateIdentity{}}
}

func (s presentAssignmentState) len() int { return len(s.entries) }

func (s presentAssignmentState) get(key presentAssignmentKey) (conditionalAssignment, bool) {
	assignment, ok := s.entries[key]
	return assignment, ok
}

// presentAssignmentTransferStats is optional test/benchmark accounting for the
// small assignment dataflow run. Production lowerers leave it nil.
type presentAssignmentTransferStats struct {
	transfers    int
	stateClones  int
	reusedStates int
	mutations    int
}

// presentAssignmentStateEditor owns copy-on-first-write mutation for one
// transfer. Its input may be retained by an in-state, an out-state, and sibling
// CFG edges; ensureMutable is therefore the only legal path to map mutation.
type presentAssignmentStateEditor struct {
	state  presentAssignmentState
	cloned bool
	stats  *presentAssignmentTransferStats
}

func (e *presentAssignmentStateEditor) ensureMutable() {
	if e.cloned {
		return
	}
	entries := make(map[presentAssignmentKey]conditionalAssignment, e.state.len())
	for key, assignment := range e.state.entries {
		entries[key] = assignment
	}
	e.state = presentAssignmentState{entries: entries, identity: &presentAssignmentStateIdentity{}}
	e.cloned = true
	if e.stats != nil {
		e.stats.stateClones++
	}
}

func (e *presentAssignmentStateEditor) invalidate(written path.Path) {
	if e.state.len() == 0 || written.Symbol == 0 {
		return
	}
	hasOverlap := false
	for _, assignment := range e.state.entries {
		if assignment.path.Overlaps(written) {
			hasOverlap = true
			break
		}
	}
	if !hasOverlap {
		return
	}
	e.ensureMutable()
	for key, assignment := range e.state.entries {
		if assignment.path.Overlaps(written) {
			delete(e.state.entries, key)
			if e.stats != nil {
				e.stats.mutations++
			}
		}
	}
}

func (e *presentAssignmentStateEditor) write(key presentAssignmentKey, assignment conditionalAssignment) {
	e.ensureMutable()
	assignment.path = assignment.path.Clone()
	e.state.entries[key] = assignment
	if e.stats != nil {
		e.stats.mutations++
	}
}

func (l *lowerer) presentAssignmentsOnBranchEdge(
	input *factflow.FactsInput,
	graph cfg.Graph,
	start cfg.Point,
	join cfg.Point,
	initial presentAssignmentState,
) presentAssignmentState {
	region := branchRegion(graph, start, join)
	if len(region) == 0 {
		return presentAssignmentState{}
	}
	in := make(map[cfg.Point]presentAssignmentState, len(region))
	out := make(map[cfg.Point]presentAssignmentState, len(region))
	in[start] = clonePresentAssignmentState(initial)
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			if !region[point] {
				continue
			}
			nextIn, ok := incomingPresentAssignmentState(graph, region, out, point, start)
			if point == start {
				nextIn, ok = clonePresentAssignmentState(initial), true
			}
			if !ok {
				continue
			}
			if prior, ok := in[point]; !ok || !presentAssignmentStateEqual(prior, nextIn) {
				in[point] = nextIn
				changed = true
			}
			nextOut := l.transferPresentAssignmentState(input, point, nextIn, true)
			if prior, ok := out[point]; !ok || !presentAssignmentStateEqual(prior, nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	joined, ok := presentAssignmentStateAtRegionExit(graph, region, out, join)
	if !ok || joined.len() == 0 {
		return presentAssignmentState{}
	}
	return clonePresentAssignmentState(joined)
}

func (l *lowerer) assignmentStateBeforePoint(
	input *factflow.FactsInput,
	graph cfg.Graph,
	stop cfg.Point,
) presentAssignmentState {
	if input == nil || graph == nil || stop == graph.Entry() {
		return presentAssignmentState{}
	}
	_, out := l.solvePresentAssignmentStates(input, graph, nil, -1, stop, nil)
	nextIn, ok := incomingPresentAssignmentState(graph, nil, out, stop, graph.Entry())
	if !ok {
		return presentAssignmentState{}
	}
	return nextIn
}

// solvePresentAssignmentStates runs the assignment must-analysis over either
// the whole graph (component == nil) or one SCC. stop is omitted entirely. For
// an SCC solve, outsideOut supplies the already-solved incoming boundary.
func (l *lowerer) solvePresentAssignmentStates(
	input *factflow.FactsInput,
	graph cfg.Graph,
	componentOf map[cfg.Point]int,
	componentIndex int,
	stop cfg.Point,
	outsideOut map[cfg.Point]presentAssignmentState,
) (map[cfg.Point]presentAssignmentState, map[cfg.Point]presentAssignmentState) {
	in := make(map[cfg.Point]presentAssignmentState)
	out := make(map[cfg.Point]presentAssignmentState)
	changed := true
	for changed {
		changed = false
		for _, point := range cfg.RPOReadOnly(graph) {
			if componentIndex >= 0 && componentOf[point] != componentIndex {
				continue
			}
			if point == stop {
				continue
			}
			nextIn, ok := incomingPresentAssignmentStateWithBoundary(graph, componentOf, componentIndex, out, outsideOut, point)
			if point == graph.Entry() {
				nextIn, ok = presentAssignmentState{}, true
			}
			if !ok {
				continue
			}
			if prior, ok := in[point]; !ok || !presentAssignmentStateEqual(prior, nextIn) {
				in[point] = nextIn
				changed = true
			}
			nextOut := l.transferPresentAssignmentState(input, point, nextIn, false)
			if prior, ok := out[point]; !ok || !presentAssignmentStateEqual(prior, nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	return in, out
}

func (l *lowerer) assignmentStateBeforePointInComponent(
	input *factflow.FactsInput,
	graph cfg.Graph,
	stop cfg.Point,
	componentOf map[cfg.Point]int,
	componentIndex int,
	outsideOut map[cfg.Point]presentAssignmentState,
) presentAssignmentState {
	if stop == graph.Entry() {
		return presentAssignmentState{}
	}
	_, out := l.solvePresentAssignmentStates(input, graph, componentOf, componentIndex, stop, outsideOut)
	nextIn, ok := incomingPresentAssignmentStateWithBoundary(graph, componentOf, componentIndex, out, outsideOut, stop)
	if !ok {
		return presentAssignmentState{}
	}
	return nextIn
}

func incomingPresentAssignmentStateWithBoundary(
	graph cfg.Graph,
	componentOf map[cfg.Point]int,
	componentIndex int,
	localOut map[cfg.Point]presentAssignmentState,
	outsideOut map[cfg.Point]presentAssignmentState,
	point cfg.Point,
) (presentAssignmentState, bool) {
	var merged presentAssignmentState
	seen := false
	for _, pred := range cfg.PredecessorsReadOnly(graph, point) {
		states := localOut
		if componentIndex >= 0 && componentOf[pred] != componentIndex {
			states = outsideOut
		}
		state, ok := states[pred]
		if !ok {
			continue
		}
		if !seen {
			merged = clonePresentAssignmentState(state)
			seen = true
			continue
		}
		merged = intersectPresentAssignmentState(merged, state)
	}
	return merged, seen
}

// presentAssignmentStrongComponents returns deterministic Tarjan SCCs for the
// reachable graph. SCC identity is internal; point order within a component is
// inherited from Tarjan and never affects transfer order.
func presentAssignmentStrongComponents(
	graph cfg.Graph,
	rpo []cfg.Point,
) ([][]cfg.Point, map[cfg.Point]int) {
	index := 0
	indices := make(map[cfg.Point]int, len(rpo))
	lowlink := make(map[cfg.Point]int, len(rpo))
	onStack := make(map[cfg.Point]bool, len(rpo))
	stack := make([]cfg.Point, 0, len(rpo))
	components := make([][]cfg.Point, 0)
	componentOf := make(map[cfg.Point]int, len(rpo))

	var visit func(cfg.Point)
	visit = func(point cfg.Point) {
		indices[point] = index
		lowlink[point] = index
		index++
		stack = append(stack, point)
		onStack[point] = true
		for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
			if _, seen := indices[successor]; !seen {
				visit(successor)
				if lowlink[successor] < lowlink[point] {
					lowlink[point] = lowlink[successor]
				}
			} else if onStack[successor] && indices[successor] < lowlink[point] {
				lowlink[point] = indices[successor]
			}
		}
		if lowlink[point] != indices[point] {
			return
		}
		componentIndex := len(components)
		component := make([]cfg.Point, 0, 1)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			componentOf[member] = componentIndex
			component = append(component, member)
			if member == point {
				break
			}
		}
		components = append(components, component)
	}
	for _, point := range rpo {
		if _, seen := indices[point]; !seen {
			visit(point)
		}
	}
	return components, componentOf
}

func presentAssignmentComponentIsCyclic(graph cfg.Graph, component []cfg.Point) bool {
	if len(component) > 1 {
		return true
	}
	if len(component) == 0 {
		return false
	}
	point := component[0]
	for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
		if successor == point {
			return true
		}
	}
	return false
}

func branchRegion(graph cfg.Graph, start cfg.Point, join cfg.Point) map[cfg.Point]bool {
	if graph == nil || start == join {
		return nil
	}
	region := make(map[cfg.Point]bool)
	queue := []cfg.Point{start}
	for len(queue) != 0 {
		point := queue[0]
		queue = queue[1:]
		if point == join || region[point] {
			continue
		}
		region[point] = true
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if succ != join && !region[succ] {
				queue = append(queue, succ)
			}
		}
	}
	return region
}

func incomingPresentAssignmentState(
	graph cfg.Graph,
	region map[cfg.Point]bool,
	out map[cfg.Point]presentAssignmentState,
	point cfg.Point,
	start cfg.Point,
) (presentAssignmentState, bool) {
	if point == start {
		return presentAssignmentState{}, true
	}
	var merged presentAssignmentState
	seen := false
	for _, pred := range cfg.PredecessorsReadOnly(graph, point) {
		if region != nil && !region[pred] {
			continue
		}
		state, ok := out[pred]
		if !ok {
			continue
		}
		if !seen {
			merged = clonePresentAssignmentState(state)
			seen = true
			continue
		}
		merged = intersectPresentAssignmentState(merged, state)
	}
	return merged, seen
}

func presentAssignmentStateAtRegionExit(
	graph cfg.Graph,
	region map[cfg.Point]bool,
	out map[cfg.Point]presentAssignmentState,
	join cfg.Point,
) (presentAssignmentState, bool) {
	var merged presentAssignmentState
	seen := false
	for _, pred := range cfg.PredecessorsReadOnly(graph, join) {
		if !region[pred] {
			continue
		}
		state, ok := out[pred]
		if !ok {
			continue
		}
		if !seen {
			merged = clonePresentAssignmentState(state)
			seen = true
			continue
		}
		merged = intersectPresentAssignmentState(merged, state)
	}
	return merged, seen
}

func (l *lowerer) transferPresentAssignmentState(
	input *factflow.FactsInput,
	point cfg.Point,
	in presentAssignmentState,
	fromBranch bool,
) presentAssignmentState {
	stats := l.presentAssignmentStats
	if stats != nil {
		stats.transfers++
	}
	editor := presentAssignmentStateEditor{state: in, stats: stats}
	if fact, ok := input.RootAssignments[point]; ok {
		target := fact.TargetPathRef()
		if target.Symbol != 0 {
			editor.invalidate(target)
			if len(target.Segments) == 0 {
				l.writePresentAssignmentState(input, &editor, target, fact.Source(), fromBranch)
			}
		}
	}
	if fact, ok := input.PathAssignments[point]; ok {
		target := fact.TargetPathRef()
		if target.Symbol != 0 {
			editor.invalidate(target)
			l.writePresentAssignmentState(input, &editor, target, fact.Source(), fromBranch)
		}
	}
	if !editor.cloned && stats != nil {
		stats.reusedStates++
	}
	return editor.state
}

func (l *lowerer) writePresentAssignmentState(
	input *factflow.FactsInput,
	editor *presentAssignmentStateEditor,
	target path.Path,
	source factflow.ValueSource,
	fromBranch bool,
) {
	key, ok := presentAssignmentKeyForPath(target)
	if !ok {
		return
	}
	if value, ok := l.rootAssignmentSourceValue(input, source); ok {
		editor.write(key, conditionalAssignment{path: target, value: value, hasValue: true, fromBranch: fromBranch})
	} else if l.rootAssignmentSourceDefinitelyPresent(input, source) {
		editor.write(key, conditionalAssignment{path: target, fromBranch: fromBranch})
	}
}

func presentAssignmentKeyForPath(p path.Path) (presentAssignmentKey, bool) {
	if p.Symbol == 0 {
		return "", false
	}
	return presentAssignmentKey(p.Key()), true
}

func (l *lowerer) rootAssignmentSourceDefinitelyPresent(input *factflow.FactsInput, source factflow.ValueSource) bool {
	if value, ok := l.rootAssignmentSourceValue(input, source); ok {
		return presence.Equal(product.PresenceOf(value), presence.Present())
	}
	switch source.Kind {
	case factflow.ValueSourceNil, factflow.ValueSourceUnknown, factflow.ValueSourceVararg:
		return false
	default:
		return false
	}
}

func (l *lowerer) rootAssignmentSourceValue(input *factflow.FactsInput, source factflow.ValueSource) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		value, ok := l.expressionValues[source.ExprRef]
		if !ok || !usefulConditionalAssignmentValue(l.registry, value) {
			return product.Value{}, false
		}
		return value, true
	case factflow.ValueSourceCall:
		return l.callSourceValue(input, source)
	case factflow.ValueSourceLiteral:
		value, ok := l.literalSourceValue(source)
		if !ok || !usefulConditionalAssignmentValue(l.registry, value) {
			return product.Value{}, false
		}
		return value, true
	default:
		return product.Value{}, false
	}
}

func (l *lowerer) literalSourceValue(source factflow.ValueSource) (product.Value, bool) {
	switch source.LiteralKind {
	case factflow.ValueSourceLiteralBool:
		return l.valueFromTypeWithWitness(typ.LiteralBool(source.Bool)), true
	case factflow.ValueSourceLiteralInteger:
		return l.valueFromTypeWithWitness(typ.LiteralInt(source.Int)), true
	case factflow.ValueSourceLiteralNumber:
		return l.valueFromTypeWithWitness(typ.LiteralNumber(source.Float)), true
	case factflow.ValueSourceLiteralString:
		return l.valueFromTypeWithWitness(typ.LiteralString(source.String)), true
	default:
		return product.Value{}, false
	}
}

func (l *lowerer) callSourceValue(input *factflow.FactsInput, source factflow.ValueSource) (product.Value, bool) {
	if input == nil || !source.HasCallPoint || source.ResultIndex > 0 {
		return product.Value{}, false
	}
	site, ok := input.CallSites[source.CallPoint]
	if !ok {
		return product.Value{}, false
	}
	t, ok := l.callSiteFirstReturnType(site.View())
	if !ok {
		return product.Value{}, false
	}
	value := l.valueFromTypeWithWitness(t)
	if !usefulConditionalAssignmentValue(l.registry, value) {
		return product.Value{}, false
	}
	return value, true
}

func usefulConditionalAssignmentValue(reg *axis.Registry, value product.Value) bool {
	if reg == nil || value == product.Top() || product.Equal(reg, value, product.Bottom(reg)) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return false
	}
	return true
}

func (l *lowerer) callSiteFirstReturnType(site factflow.CallSiteView) (typ.Type, bool) {
	return l.callSiteReturnType(site, 0)
}

func (l *lowerer) callSiteReturnType(site factflow.CallSiteView, index int) (typ.Type, bool) {
	return l.callSiteReturnTypeAt(0, site, index)
}

func (l *lowerer) callSiteReturnTypeAt(point cfg.Point, site factflow.CallSiteView, index int) (typ.Type, bool) {
	if l != nil && l.wir != nil && point != 0 {
		if inst, ok := l.wirCallInstruction(point); ok && inst.Call.Method == 0 && inst.Type != 0 {
			if callable, ok := typecall.Callable(l.wir.Type(inst.Type)); ok && callable != nil &&
				index >= 0 && index < len(callable.Returns) && callable.Returns[index] != nil {
				return callable.Returns[index], true
			}
		}
	}
	calleeType, ok := l.callSiteCalleeType(site)
	if !ok {
		return nil, false
	}
	callable, ok := typecall.Callable(calleeType)
	if !ok || callable == nil || index < 0 || index >= len(callable.Returns) || callable.Returns[index] == nil {
		return nil, false
	}
	return callable.Returns[index], true
}

func (l *lowerer) callSiteCalleeType(site factflow.CallSiteView) (typ.Type, bool) {
	if site.CalleeSymbol() != 0 && !site.CalleeMemberAccess() {
		if t, ok := l.symbolTypes[site.CalleeSymbol()]; ok {
			return t, true
		}
	}
	calleePath := site.CalleePathRef()
	if calleePath.Symbol != 0 {
		root, ok := l.symbolTypes[calleePath.Symbol]
		if !ok {
			return nil, false
		}
		if len(calleePath.Segments) == 0 {
			return root, true
		}
		return luatypeprojection.ApplySegments(root, calleePath.Segments)
	}
	return nil, false
}

func clonePresentAssignmentState(in presentAssignmentState) presentAssignmentState {
	// States are immutable after publication. Cloning is therefore ownership
	// transfer, not data copying; the editor performs copy-on-first-write.
	return in
}

func intersectPresentAssignmentState(a, b presentAssignmentState) presentAssignmentState {
	if a.len() == 0 || b.len() == 0 {
		return presentAssignmentState{}
	}
	if a.identity != nil && a.identity == b.identity {
		return a
	}
	retained := 0
	reusesB := true
	for key, left := range a.entries {
		right, ok := b.entries[key]
		if ok && conditionalAssignmentsEqual(left, right) {
			retained++
			if !conditionalAssignmentRepresentationEqual(left, right) {
				reusesB = false
			}
		}
	}
	if retained == 0 {
		return presentAssignmentState{}
	}
	if retained == a.len() {
		return a
	}
	if retained == b.len() && reusesB {
		return b
	}
	entries := make(map[presentAssignmentKey]conditionalAssignment, retained)
	for key, left := range a.entries {
		right, ok := b.entries[key]
		if ok && conditionalAssignmentsEqual(left, right) {
			entries[key] = left
		}
	}
	return newPresentAssignmentState(entries)
}

func presentAssignmentStateEqual(a, b presentAssignmentState) bool {
	if a.identity != nil && a.identity == b.identity {
		return true
	}
	if a.len() != b.len() {
		return false
	}
	for key, left := range a.entries {
		right, ok := b.entries[key]
		if !ok || !conditionalAssignmentsEqual(left, right) {
			return false
		}
	}
	return true
}

func conditionalAssignmentsEqual(a, b conditionalAssignment) bool {
	if !a.path.Equal(b.path) || a.hasValue != b.hasValue {
		return false
	}
	if !a.hasValue {
		return true
	}
	return a.value == b.value
}

func conditionalAssignmentRepresentationEqual(a, b conditionalAssignment) bool {
	return conditionalAssignmentsEqual(a, b) && a.fromBranch == b.fromBranch
}
