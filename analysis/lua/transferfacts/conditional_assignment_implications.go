package transferfacts

import (
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

// MaxPartitionImplicationsPerBody bounds discriminant-keyed conditional facts
// produced by this lowering pass. Overflow simply stops publishing additional
// facts; ordinary unpartitioned flow remains sound.
const MaxPartitionImplicationsPerBody = 64

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
	partitionImplicationCount := 0
	l.addChannelSelectPayloadImplications(input, graph, &partitionImplicationCount)
	if graph == nil || len(input.RootAssignments) == 0 {
		return
	}
	postdom := dominance.ComputeImmediatePostDominators(graph)
	if len(postdom) == 0 {
		return
	}
	rpo := graph.RPO()
	continuationJoins := newBranchContinuationJoinCache(graph, rpo)
	for _, branch := range rpo {
		if partitionImplicationCount >= MaxPartitionImplicationsPerBody {
			return
		}
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
		incoming := l.assignmentStateBeforePoint(input, graph, branch)
		edgeAssignments := make(map[cfg.Point]presentAssignmentState)
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			edgeAssignments[succ] = l.presentAssignmentsOnBranchEdge(input, graph, succ, join, incoming)
		}
		for succ, selected := range edgeAssignments {
			l.addBranchAssignmentValueImplications(input, join, incoming, selected, oppositeBranchAssignmentStates(graph, branch, succ, edgeAssignments), &partitionImplicationCount)
			if partitionImplicationCount >= MaxPartitionImplicationsPerBody {
				return
			}
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
			if len(present) == 0 {
				continue
			}
			for _, trigger := range branchEdgeValueTriggers(l.registry, refinements, cond) {
				if !branchTriggerContradictedOnEdge(l.registry, refinements, trigger, !cond) {
					continue
				}
				for _, target := range present {
					if !appendConditionalAssignmentTargetImplication(
						input.PathValuePresenceImplications,
						join,
						&partitionImplicationCount,
						conditionalImplicationTrigger{path: trigger.path, value: trigger.value},
						target,
					) {
						return
					}
				}
			}
		}
	}
}

func (l *lowerer) addChannelSelectPayloadImplications(
	input *factflow.FactsInput,
	graph cfg.Graph,
	partitionImplicationCount *int,
) {
	if l == nil || l.registry == nil || input == nil || graph == nil {
		return
	}
	for _, point := range graph.RPO() {
		if partitionImplicationCount != nil && *partitionImplicationCount >= MaxPartitionImplicationsPerBody {
			return
		}
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
			if !appendPartitionPathValuePresenceImplication(
				input.PathValuePresenceImplications,
				point,
				partitionImplicationCount,
				factflow.NewPathEqualityValueRefinementImplication(
					resultPath.Field("channel"),
					casePath,
					resultPath.Field("value"),
					payload,
				),
			) {
				return
			}
		}
	}
}

func (l *lowerer) addBranchAssignmentValueImplications(
	input *factflow.FactsInput,
	join cfg.Point,
	incoming presentAssignmentState,
	selected presentAssignmentState,
	opposites []presentAssignmentState,
	partitionImplicationCount *int,
) {
	if input == nil || len(selected) == 0 || len(opposites) == 0 {
		return
	}
	for triggerSym, triggerAssignment := range selected {
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
		for targetSym, target := range selected {
			if targetSym == triggerSym {
				continue
			}
			if !appendConditionalAssignmentTargetImplication(input.PathValuePresenceImplications, join, partitionImplicationCount, trigger, target) {
				return
			}
		}
	}
}

type conditionalImplicationTrigger struct {
	path          path.Path
	value         product.Value
	requireTruthy bool
}

func appendConditionalAssignmentTargetImplication(
	out map[cfg.Point]factflow.PathValuePresenceImplicationSet,
	point cfg.Point,
	count *int,
	trigger conditionalImplicationTrigger,
	target conditionalAssignment,
) bool {
	if target.path.Symbol == 0 || !target.fromBranch {
		return true
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
	return appendPartitionPathValuePresenceImplication(out, point, count, implication)
}

func appendPartitionPathValuePresenceImplication(
	out map[cfg.Point]factflow.PathValuePresenceImplicationSet,
	point cfg.Point,
	count *int,
	implication factflow.PathValuePresenceImplication,
) bool {
	if count != nil && *count >= MaxPartitionImplicationsPerBody {
		return false
	}
	appendPathValuePresenceImplications(out, point, implication)
	if count != nil {
		(*count)++
	}
	return true
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
		other, ok := opposite[triggerKey]
		if !ok || !other.hasValue {
			other, ok = incoming[triggerKey]
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
	baseline, ok := incoming[triggerKey]
	if !ok || !baseline.hasValue {
		return false
	}
	for _, opposite := range opposites {
		if other, ok := opposite[triggerKey]; ok && other.fromBranch {
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

type presentAssignmentState map[presentAssignmentKey]conditionalAssignment

func (l *lowerer) presentAssignmentsOnBranchEdge(
	input *factflow.FactsInput,
	graph cfg.Graph,
	start cfg.Point,
	join cfg.Point,
	initial presentAssignmentState,
) presentAssignmentState {
	region := branchRegion(graph, start, join)
	if len(region) == 0 {
		return nil
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
	if !ok || len(joined) == 0 {
		return nil
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
	in := make(map[cfg.Point]presentAssignmentState)
	out := make(map[cfg.Point]presentAssignmentState)
	in[graph.Entry()] = presentAssignmentState{}
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			if point == stop {
				continue
			}
			nextIn, ok := incomingPresentAssignmentState(graph, nil, out, point, graph.Entry())
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
	nextIn, ok := incomingPresentAssignmentState(graph, nil, out, stop, graph.Entry())
	if !ok {
		return presentAssignmentState{}
	}
	return nextIn
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
	out := clonePresentAssignmentState(in)
	if fact, ok := input.RootAssignments[point]; ok {
		target := fact.TargetPath()
		if target.Symbol != 0 {
			out = invalidatePresentAssignmentPath(out, target)
			if len(target.Segments) == 0 {
				out = l.writePresentAssignmentState(input, out, target, fact.Source(), fromBranch)
			}
		}
	}
	if fact, ok := input.PathAssignments[point]; ok {
		target := fact.TargetPath()
		if target.Symbol != 0 {
			out = invalidatePresentAssignmentPath(out, target)
			out = l.writePresentAssignmentState(input, out, target, fact.Source(), fromBranch)
		}
	}
	return out
}

func (l *lowerer) writePresentAssignmentState(
	input *factflow.FactsInput,
	out presentAssignmentState,
	target path.Path,
	source factflow.ValueSource,
	fromBranch bool,
) presentAssignmentState {
	key, ok := presentAssignmentKeyForPath(target)
	if !ok {
		return out
	}
	if value, ok := l.rootAssignmentSourceValue(input, source); ok {
		out[key] = conditionalAssignment{path: target, value: value, hasValue: true, fromBranch: fromBranch}
	} else if l.rootAssignmentSourceDefinitelyPresent(input, source) {
		out[key] = conditionalAssignment{path: target, fromBranch: fromBranch}
	}
	return out
}

func invalidatePresentAssignmentPath(in presentAssignmentState, written path.Path) presentAssignmentState {
	if len(in) == 0 || written.Symbol == 0 {
		return in
	}
	for key, assignment := range in {
		if assignment.path.Overlaps(written) {
			delete(in, key)
		}
	}
	return in
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
	if len(in) == 0 {
		return presentAssignmentState{}
	}
	out := make(presentAssignmentState, len(in))
	for key, target := range in {
		target.path = target.path.Clone()
		out[key] = target
	}
	return out
}

func intersectPresentAssignmentState(a, b presentAssignmentState) presentAssignmentState {
	if len(a) == 0 || len(b) == 0 {
		return presentAssignmentState{}
	}
	out := make(presentAssignmentState)
	for key, left := range a {
		right, ok := b[key]
		if ok && conditionalAssignmentsEqual(left, right) {
			left.path = left.path.Clone()
			out[key] = left
		}
	}
	return out
}

func presentAssignmentStateEqual(a, b presentAssignmentState) bool {
	if len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, ok := b[key]
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
