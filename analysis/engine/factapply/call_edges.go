package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type callOutcomePresenceTargets struct {
	triggerTarget factflow.CallResultTarget
	target        factflow.CallResultTarget
	triggerAssign cfg.Point
	targetAssign  cfg.Point
	establish     cfg.Point
}

func applyCallOutcomeEdgeFacts(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	outcomeProvider CallOutcomeProvider,
	resolver *visibility.Resolver,
	branchRefinements []factflow.BranchRefinement,
	out state.State,
) state.State {
	if outcomeProvider == nil || ctx.Graph == nil || !ctx.HasCond {
		return out
	}
	for _, callPoint := range ctx.Graph.RPO() {
		site, ok := facts.CallSite(callPoint)
		if !ok {
			continue
		}
		outcome := outcomeProvider(transfer.NodeContext{
			Graph:    ctx.Graph,
			Registry: ctx.Registry,
			Point:    callPoint,
			Node:     ctx.Graph.Node(callPoint),
			Read:     emptyStateRead,
		}, site, out, emptyStateRead)
		if len(outcome.ReturnConditionRefinements) != 0 {
			out = applyCallReturnConditionRefinements(ctx, facts, resolver, callPoint, site, outcome, out)
		}
		if len(outcome.ReturnPresenceRelations) != 0 {
			out = applyCallReturnPresenceRelations(ctx, facts, resolver, branchRefinements, callPoint, site, outcome, out)
		}
	}
	return out
}

func applyCallReturnConditionRefinements(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	callPoint cfg.Point,
	site factflow.CallSite,
	outcome CallOutcome,
	out state.State,
) state.State {
	if site.Context() != factflow.CallSiteContextCondition {
		return out
	}
	branch, ok := callOutcomeConditionBranchPoint(ctx.Graph, callPoint)
	if !ok || branch != ctx.Edge.From {
		return out
	}
	bindings := callArgumentPlaceholderBindings(facts, site)
	for _, refinement := range outcome.ReturnConditionRefinements {
		if refinement.ReturnIndex != 0 || refinement.ReturnValue != ctx.Edge.Cond {
			continue
		}
		targetPath, ok := refinement.Target.Substitute(bindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(
			ctx.Registry,
			resolver,
			ctx.Edge.From,
			out,
			targetPath,
			factflow.NewValueConstraint(refinement.Value),
		)
	}
	return out
}

func applyCallReturnPresenceRelations(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	branchRefinements []factflow.BranchRefinement,
	callPoint cfg.Point,
	site factflow.CallSite,
	outcome CallOutcome,
	out state.State,
) state.State {
	if !ctx.Graph.IsBranch(ctx.Edge.From) {
		return out
	}
	for _, relation := range outcome.ReturnPresenceRelations {
		out = applyCallReturnPresenceRelation(ctx, facts, resolver, branchRefinements, callPoint, site, relation, out)
	}
	return out
}

func applyCallReturnPresenceRelation(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	branchRefinements []factflow.BranchRefinement,
	callPoint cfg.Point,
	site factflow.CallSite,
	relation CallReturnPresenceRelation,
	out state.State,
) state.State {
	triggerTarget, ok := callOutcomeTargetForResult(site, relation.TriggerIndex)
	if !ok || !callOutcomeRelatableTarget(triggerTarget) {
		return out
	}
	target, ok := callOutcomeTargetForResult(site, relation.TargetIndex)
	if !ok || !callOutcomeRelatableTarget(target) {
		return out
	}
	triggerAssign, ok := callOutcomeResultAssignmentPoint(ctx.Graph, facts, callPoint, triggerTarget, relation.TriggerIndex)
	if !ok {
		return out
	}
	targetAssign, ok := callOutcomeResultAssignmentPoint(ctx.Graph, facts, callPoint, target, relation.TargetIndex)
	if !ok {
		return out
	}
	targets := callOutcomePresenceTargets{
		triggerTarget: triggerTarget,
		target:        target,
		triggerAssign: triggerAssign,
		targetAssign:  targetAssign,
		establish:     callOutcomeLaterPoint(ctx.Graph, targetAssign, triggerAssign),
	}
	activeIn := callOutcomeRelationActiveIn(ctx.Graph, facts, targets)
	if !activeIn[ctx.Edge.From] || !callOutcomeBranchRefinesPath(branchRefinements, triggerTarget) {
		return out
	}
	branchRelation := factflow.NewBranchPresenceRelation(
		triggerTarget.TargetPath(),
		relation.TriggerPresence,
		target.TargetPath(),
		relation.TargetPresence,
	)
	refinement, ok := branchPresenceRelationRefinement(ctx, resolver, out, branchRefinements, branchRelation)
	if !ok {
		return out
	}
	return applyBranchRefinement(ctx, resolver, out, branchRelation.TargetPath(), refinement)
}

func callOutcomeConditionBranchPoint(graph cfg.Graph, point cfg.Point) (cfg.Point, bool) {
	if graph == nil {
		return 0, false
	}
	if graph.IsBranch(point) {
		return point, true
	}
	successors := graph.Successors(point)
	if len(successors) != 1 {
		return 0, false
	}
	branch := successors[0]
	if !graph.IsBranch(branch) {
		return 0, false
	}
	return branch, true
}

func callOutcomeTargetForResult(site factflow.CallSite, resultIndex int) (factflow.CallResultTarget, bool) {
	if resultIndex < 0 {
		return factflow.CallResultTarget{}, false
	}
	for _, target := range site.ResultTargets() {
		if target.ResultIndex() == resultIndex {
			return target, true
		}
	}
	return factflow.CallResultTarget{}, false
}

func callOutcomeRelatableTarget(target factflow.CallResultTarget) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment, factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && !target.TargetPath().IsEmpty()
	default:
		return false
	}
}

func callOutcomeResultAssignmentPoint(
	graph cfg.Graph,
	facts factflow.Facts,
	callPoint cfg.Point,
	target factflow.CallResultTarget,
	resultIndex int,
) (cfg.Point, bool) {
	for _, point := range graph.RPO() {
		if assignment, ok := facts.RootAssignment(point); ok &&
			assignment.TargetPath().Equal(target.TargetPath()) &&
			callOutcomeValueSourceConsumesResult(assignment.Source(), callPoint, target, resultIndex) {
			return point, true
		}
	}
	return 0, false
}

func callOutcomeValueSourceConsumesResult(
	source factflow.ValueSource,
	callPoint cfg.Point,
	target factflow.CallResultTarget,
	resultIndex int,
) bool {
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != callPoint {
		return false
	}
	if source.ResultIndex != resultIndex {
		return false
	}
	return source.TargetIndex == target.Index()
}

func callOutcomeLaterPoint(graph cfg.Graph, first, second cfg.Point) cfg.Point {
	order := callOutcomePointOrder(graph)
	if order[second] > order[first] {
		return second
	}
	return first
}

func callOutcomeRelationActiveIn(
	graph cfg.Graph,
	facts factflow.Facts,
	targets callOutcomePresenceTargets,
) map[cfg.Point]bool {
	rpo := graph.RPO()
	activeIn := make(map[cfg.Point]bool, len(rpo))
	activeOut := make(map[cfg.Point]bool, len(rpo))
	for changed := true; changed; {
		changed = false
		for _, point := range rpo {
			in := callOutcomeAllPredecessorsActive(graph, point, activeOut)
			out := in
			switch {
			case point == targets.establish:
				out = true
			case in && callOutcomeRelationKilledAt(facts, point, targets):
				out = false
			}
			if activeIn[point] != in {
				activeIn[point] = in
				changed = true
			}
			if activeOut[point] != out {
				activeOut[point] = out
				changed = true
			}
		}
	}
	return activeIn
}

func callOutcomeAllPredecessorsActive(graph cfg.Graph, point cfg.Point, activeOut map[cfg.Point]bool) bool {
	preds := graph.Predecessors(point)
	if len(preds) == 0 {
		return false
	}
	for _, pred := range preds {
		if !activeOut[pred] {
			return false
		}
	}
	return true
}

func callOutcomeRelationKilledAt(
	facts factflow.Facts,
	point cfg.Point,
	targets callOutcomePresenceTargets,
) bool {
	if point == targets.targetAssign || point == targets.triggerAssign {
		return false
	}
	if assignment, ok := facts.RootAssignment(point); ok && callOutcomeRelationTargetPath(assignment.TargetPath(), targets) {
		return true
	}
	if pathAssignment, ok := facts.PathAssignment(point); ok && callOutcomeRelationTargetPath(pathAssignment.TargetPath(), targets) {
		return true
	}
	return false
}

func callOutcomeRelationTargetPath(targetPath pathdom.Path, targets callOutcomePresenceTargets) bool {
	return targetPath.Equal(targets.target.TargetPath()) || targetPath.Equal(targets.triggerTarget.TargetPath())
}

func callOutcomeBranchRefinesPath(
	branchRefinements []factflow.BranchRefinement,
	target factflow.CallResultTarget,
) bool {
	targetPath := target.TargetPath()
	for _, refinement := range branchRefinements {
		if refinement.TargetPath().Equal(targetPath) {
			return true
		}
	}
	return false
}

func callOutcomePointOrder(graph cfg.Graph) map[cfg.Point]int {
	rpo := graph.RPO()
	out := make(map[cfg.Point]int, len(rpo))
	for i, point := range rpo {
		out[point] = i
	}
	return out
}
