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
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// addConditionalAssignmentImplications derives merge-point facts of the form:
// if a later path value proves the same branch condition, then a target assigned
// on that branch edge keeps the value assigned on that edge. This preserves correlations such as
// `if not use_template then executor = make() end` across the join, while still
// requiring every path on the selected edge to end with a compatible write and
// the opposite edge to contradict the trigger value.
func (l *lowerer) addConditionalAssignmentImplications(input *factflow.FactsInput, graph cfg.Graph, result *semantics.Result) {
	if input == nil || graph == nil || result == nil || len(input.RootAssignments) == 0 {
		return
	}
	postdom := dominance.ComputeImmediatePostDominators(graph)
	if len(postdom) == 0 {
		return
	}
	rpo := graph.RPO()
	continuationJoins := newBranchContinuationJoinCache(graph, rpo)
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
		incoming := l.assignmentStateBeforePoint(input, graph, result, branch)
		edgeAssignments := make(map[cfg.Point]presentAssignmentState)
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			edgeAssignments[succ] = l.presentAssignmentsOnBranchEdge(input, graph, result, succ, join, incoming)
		}
		for succ, selected := range edgeAssignments {
			l.addBranchAssignmentValueImplications(input, join, incoming, selected, oppositeBranchAssignmentStates(graph, branch, succ, edgeAssignments))
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
					if target.path.Symbol == 0 || !target.fromBranch {
						continue
					}
					if target.hasValue {
						appendPathValuePresenceImplications(input.PathValuePresenceImplications, join,
							factflow.NewPathValueRefinementImplication(
								trigger.path,
								trigger.value,
								target.path,
								target.value,
							),
						)
					} else {
						appendPathValuePresenceImplications(input.PathValuePresenceImplications, join,
							factflow.NewPathValuePresenceImplication(
								trigger.path,
								trigger.value,
								target.path,
								presence.Present(),
							),
						)
					}
				}
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
) {
	if input == nil || len(selected) == 0 || len(opposites) == 0 {
		return
	}
	for triggerSym, trigger := range selected {
		if !trigger.fromBranch || !trigger.hasValue {
			continue
		}
		if _, ok := literalBoolValue(l.registry, trigger.value); !ok {
			continue
		}
		if !oppositeAssignmentsContradictTrigger(l.registry, triggerSym, trigger, incoming, opposites) {
			continue
		}
		for targetSym, target := range selected {
			if targetSym == triggerSym || !target.fromBranch || target.path.Symbol == 0 {
				continue
			}
			if target.hasValue {
				appendPathValuePresenceImplications(input.PathValuePresenceImplications, join,
					factflow.NewPathTruthyValueRefinementImplication(
						trigger.path,
						trigger.value,
						target.path,
						target.value,
					),
				)
			} else {
				appendPathValuePresenceImplications(input.PathValuePresenceImplications, join,
					factflow.NewPathValuePresenceImplication(
						trigger.path,
						trigger.value,
						target.path,
						presence.Present(),
					),
				)
			}
		}
	}
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
	triggerSym symbol.ID,
	trigger conditionalAssignment,
	incoming presentAssignmentState,
	opposites []presentAssignmentState,
) bool {
	if reg == nil || len(opposites) == 0 {
		return false
	}
	if incomingTriggerContradictsWithoutOppositeBranchWrite(reg, triggerSym, trigger, incoming, opposites) {
		return true
	}
	for _, opposite := range opposites {
		other, ok := opposite[triggerSym]
		if !ok || !other.hasValue {
			other, ok = incoming[triggerSym]
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
	triggerSym symbol.ID,
	trigger conditionalAssignment,
	incoming presentAssignmentState,
	opposites []presentAssignmentState,
) bool {
	baseline, ok := incoming[triggerSym]
	if !ok || !baseline.hasValue {
		return false
	}
	for _, opposite := range opposites {
		if other, ok := opposite[triggerSym]; ok && other.fromBranch {
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

type presentAssignmentState map[symbol.ID]conditionalAssignment

func (l *lowerer) presentAssignmentsOnBranchEdge(
	input *factflow.FactsInput,
	graph cfg.Graph,
	result *semantics.Result,
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
			nextOut := l.transferPresentAssignmentState(input, result, point, nextIn, true)
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
	result *semantics.Result,
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
			nextOut := l.transferPresentAssignmentState(input, result, point, nextIn, false)
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
	result *semantics.Result,
	point cfg.Point,
	in presentAssignmentState,
	fromBranch bool,
) presentAssignmentState {
	out := clonePresentAssignmentState(in)
	if fact, ok := input.RootAssignments[point]; ok {
		target := fact.TargetPath()
		if target.Symbol != 0 && len(target.Segments) == 0 {
			if value, ok := l.rootAssignmentSourceValue(result, fact.Source()); ok {
				out[target.Symbol] = conditionalAssignment{path: target, value: value, hasValue: true, fromBranch: fromBranch}
			} else if l.rootAssignmentSourceDefinitelyPresent(result, fact.Source()) {
				out[target.Symbol] = conditionalAssignment{path: target, fromBranch: fromBranch}
			} else {
				delete(out, target.Symbol)
			}
		}
	}
	return out
}

func (l *lowerer) rootAssignmentSourceDefinitelyPresent(result *semantics.Result, source factflow.ValueSource) bool {
	if value, ok := l.rootAssignmentSourceValue(result, source); ok {
		return presence.Equal(product.PresenceOf(value), presence.Present())
	}
	switch source.Kind {
	case factflow.ValueSourceNil, factflow.ValueSourceUnknown, factflow.ValueSourceVararg:
		return false
	default:
		return false
	}
}

func (l *lowerer) rootAssignmentSourceValue(result *semantics.Result, source factflow.ValueSource) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		value, ok := l.expressionValues[source.ExprRef]
		if !ok || !usefulConditionalAssignmentValue(l.registry, value) {
			return product.Value{}, false
		}
		return value, true
	case factflow.ValueSourceCall:
		return l.callSourceValue(result, source)
	default:
		return product.Value{}, false
	}
}

func (l *lowerer) callSourceValue(result *semantics.Result, source factflow.ValueSource) (product.Value, bool) {
	if result == nil || !source.HasCallPoint || source.ResultIndex > 0 {
		return product.Value{}, false
	}
	view, ok := result.CallView(source.CallPoint)
	if !ok {
		return product.Value{}, false
	}
	fact, _ := view.Borrowed()
	t, ok := l.callFirstReturnType(fact)
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

func (l *lowerer) callFirstReturnType(fact semantics.CallFact) (typ.Type, bool) {
	calleeType, ok := l.callCalleeType(fact)
	if !ok {
		return nil, false
	}
	callable, ok := typecall.Callable(calleeType)
	if !ok || callable == nil || len(callable.Returns) == 0 || callable.Returns[0] == nil {
		return nil, false
	}
	return callable.Returns[0], true
}

func (l *lowerer) callCalleeType(fact semantics.CallFact) (typ.Type, bool) {
	if fact.HasCalleeSymbol && fact.CalleeSymbol != 0 && !fact.CalleeMemberAccess {
		if t, ok := l.symbolTypes[fact.CalleeSymbol]; ok {
			return t, true
		}
	}
	if fact.HasCalleePath && fact.CalleePath.Symbol != 0 {
		root, ok := l.symbolTypes[fact.CalleePath.Symbol]
		if !ok {
			return nil, false
		}
		if len(fact.CalleePath.Segments) == 0 {
			return root, true
		}
		return luatypeprojection.ApplySegments(root, fact.CalleePath.Segments)
	}
	return nil, false
}

func clonePresentAssignmentState(in presentAssignmentState) presentAssignmentState {
	if len(in) == 0 {
		return presentAssignmentState{}
	}
	out := make(presentAssignmentState, len(in))
	for sym, target := range in {
		target.path = target.path.Clone()
		out[sym] = target
	}
	return out
}

func intersectPresentAssignmentState(a, b presentAssignmentState) presentAssignmentState {
	if len(a) == 0 || len(b) == 0 {
		return presentAssignmentState{}
	}
	out := make(presentAssignmentState)
	for sym, left := range a {
		right, ok := b[sym]
		if ok && conditionalAssignmentsEqual(left, right) {
			left.path = left.path.Clone()
			out[sym] = left
		}
	}
	return out
}

func presentAssignmentStateEqual(a, b presentAssignmentState) bool {
	if len(a) != len(b) {
		return false
	}
	for sym, left := range a {
		right, ok := b[sym]
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
