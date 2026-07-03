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
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// addConditionalAssignmentImplications derives merge-point facts of the form:
// if a later path value proves the same branch condition, then a target assigned
// on that branch edge is present. This preserves correlations such as
// `if not use_template then executor = make() end` across the join, while still
// requiring every path on the selected edge to end with a definitely-present
// write and the opposite edge to contradict the trigger value.
func (l *lowerer) addConditionalAssignmentImplications(input *factflow.FactsInput, graph cfg.Graph, result *semantics.Result) {
	if input == nil || graph == nil || result == nil || len(input.RootAssignments) == 0 {
		return
	}
	postdom := dominance.ComputeImmediatePostDominators(graph)
	if len(postdom) == 0 {
		return
	}
	for _, branch := range graph.RPO() {
		if !graph.IsBranch(branch) {
			continue
		}
		join, ok := postdom[branch]
		if !ok || join == branch || join == graph.Exit() {
			continue
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
			present := l.presentAssignmentsOnBranchEdge(input, graph, result, succ, join)
			if len(present) == 0 {
				continue
			}
			for _, trigger := range branchEdgeValueTriggers(l.registry, refinements, cond) {
				if !branchTriggerContradictedOnEdge(l.registry, refinements, trigger, !cond) {
					continue
				}
				for _, target := range present {
					if target.Symbol == 0 {
						continue
					}
					appendPathValuePresenceImplications(input.PathValuePresenceImplications, join,
						factflow.NewPathValuePresenceImplication(
							trigger.path,
							trigger.value,
							target,
							presence.Present(),
						),
					)
				}
			}
		}
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

type presentAssignmentState map[symbol.ID]path.Path

func (l *lowerer) presentAssignmentsOnBranchEdge(
	input *factflow.FactsInput,
	graph cfg.Graph,
	result *semantics.Result,
	start cfg.Point,
	join cfg.Point,
) []path.Path {
	region := branchRegion(graph, start, join)
	if len(region) == 0 {
		return nil
	}
	in := make(map[cfg.Point]presentAssignmentState, len(region))
	out := make(map[cfg.Point]presentAssignmentState, len(region))
	in[start] = presentAssignmentState{}
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			if !region[point] {
				continue
			}
			nextIn, ok := incomingPresentAssignmentState(graph, region, out, point, start)
			if !ok {
				continue
			}
			if prior, ok := in[point]; !ok || !presentAssignmentStateEqual(prior, nextIn) {
				in[point] = nextIn
				changed = true
			}
			nextOut := l.transferPresentAssignmentState(input, result, point, nextIn)
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
	targets := make([]path.Path, 0, len(joined))
	for _, target := range joined {
		targets = append(targets, target.Clone())
	}
	return targets
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
) presentAssignmentState {
	out := clonePresentAssignmentState(in)
	if fact, ok := input.RootAssignments[point]; ok {
		target := fact.TargetPath()
		if target.Symbol != 0 && len(target.Segments) == 0 {
			if l.rootAssignmentSourceDefinitelyPresent(result, fact.Source()) {
				out[target.Symbol] = target
			} else {
				delete(out, target.Symbol)
			}
		}
	}
	return out
}

func (l *lowerer) rootAssignmentSourceDefinitelyPresent(result *semantics.Result, source factflow.ValueSource) bool {
	switch source.Kind {
	case factflow.ValueSourceNil, factflow.ValueSourceUnknown, factflow.ValueSourceVararg:
		return false
	case factflow.ValueSourceExpression:
		value, ok := l.expressionValues[source.ExprRef]
		return ok && presence.Equal(product.PresenceOf(value), presence.Present())
	case factflow.ValueSourceCall:
		return l.callSourceDefinitelyPresent(result, source)
	default:
		return false
	}
}

func (l *lowerer) callSourceDefinitelyPresent(result *semantics.Result, source factflow.ValueSource) bool {
	if result == nil || !source.HasCallPoint || source.ResultIndex > 0 {
		return false
	}
	view, ok := result.CallView(source.CallPoint)
	if !ok {
		return false
	}
	fact, _ := view.Borrowed()
	t, ok := l.callFirstReturnType(fact)
	return ok && returnTypeProvesPresent(t)
}

func returnTypeProvesPresent(t typ.Type) bool {
	return returnTypeHasConcreteTopShape(t) && !typevalue.ProjectionHasNil(t)
}

func returnTypeHasConcreteTopShape(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil || typ.IsNever(t) || typ.IsAny(t) || typ.IsUnknown(t) {
		return false
	}
	if union, ok := t.(*typ.Union); ok {
		for _, member := range union.Members {
			if !returnTypeHasConcreteTopShape(member) {
				return false
			}
		}
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
	if fact.HasCalleeSymbol && fact.CalleeSymbol != 0 {
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
		out[sym] = target.Clone()
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
		if ok && left.Equal(right) {
			out[sym] = left.Clone()
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
		if !ok || !left.Equal(right) {
			return false
		}
	}
	return true
}
