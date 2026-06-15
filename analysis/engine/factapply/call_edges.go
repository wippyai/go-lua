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
	callPoint     cfg.Point
	triggerTarget factflow.CallResultTargetView
	target        factflow.CallResultTargetView
	triggerAssign cfg.Point
	targetAssign  cfg.Point
	establish     cfg.Point
}

type callOutcomeAssignmentKey struct {
	callPoint   cfg.Point
	resultIndex int
	targetIndex int
	targetPath  pathdom.PathKey
}

type callOutcomeActiveInKey struct {
	callPoint     cfg.Point
	triggerIndex  int
	targetIndex   int
	triggerAssign cfg.Point
	targetAssign  cfg.Point
	triggerPath   pathdom.PathKey
	targetPath    pathdom.PathKey
}

type callOutcomeTraversalCache struct {
	rpo                []cfg.Point
	pointOrder         map[cfg.Point]int
	targetsByCallPoint map[cfg.Point][]factflow.CallResultTargetView
	assignmentPoints   map[callOutcomeAssignmentKey]cfg.Point
	activeIn           map[callOutcomeActiveInKey]map[cfg.Point]bool
}

func (c *callOutcomeTraversalCache) graphRPO(graph cfg.Graph) []cfg.Point {
	if graph == nil {
		return nil
	}
	if c.rpo == nil {
		c.rpo = graph.RPO()
	}
	return c.rpo
}

func (c *callOutcomeTraversalCache) graphPointOrder(graph cfg.Graph) map[cfg.Point]int {
	if graph == nil {
		return nil
	}
	if c.pointOrder == nil {
		rpo := c.graphRPO(graph)
		c.pointOrder = make(map[cfg.Point]int, len(rpo))
		for i, point := range rpo {
			c.pointOrder[point] = i
		}
	}
	return c.pointOrder
}

func (c *callOutcomeTraversalCache) resultTargets(callPoint cfg.Point, site factflow.CallSiteView) []factflow.CallResultTargetView {
	if c.targetsByCallPoint != nil {
		if targets, ok := c.targetsByCallPoint[callPoint]; ok {
			return targets
		}
	} else {
		c.targetsByCallPoint = make(map[cfg.Point][]factflow.CallResultTargetView)
	}
	if site.ResultTargetCount() == 0 {
		c.targetsByCallPoint[callPoint] = nil
		return nil
	}
	targets := make([]factflow.CallResultTargetView, 0, site.ResultTargetCount())
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		targets = append(targets, target)
		return true
	})
	c.targetsByCallPoint[callPoint] = targets
	return targets
}

func callOutcomeTargetForResult(targets []factflow.CallResultTargetView, resultIndex int) (factflow.CallResultTargetView, bool) {
	if resultIndex < 0 {
		return factflow.CallResultTargetView{}, false
	}
	for _, target := range targets {
		if target.ResultIndex() == resultIndex {
			return target, true
		}
	}
	return factflow.CallResultTargetView{}, false
}

func callOutcomeRelatableTarget(target factflow.CallResultTargetView) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment, factflow.CallResultTargetOrdinaryAssignment:
		return target.TargetSymbol() != 0 && !target.TargetPathEmpty()
	default:
		return false
	}
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
	cache := &callOutcomeTraversalCache{}
	for _, callPoint := range cache.graphRPO(ctx.Graph) {
		siteView, ok := facts.CallSiteView(callPoint)
		if !ok {
			continue
		}
		targets := cache.resultTargets(callPoint, siteView)
		site := siteView.CallSite()
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
			out = applyCallReturnPresenceRelations(ctx, facts, cache, resolver, branchRefinements, callPoint, targets, outcome, out)
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
	cache *callOutcomeTraversalCache,
	resolver *visibility.Resolver,
	branchRefinements []factflow.BranchRefinement,
	callPoint cfg.Point,
	targets []factflow.CallResultTargetView,
	outcome CallOutcome,
	out state.State,
) state.State {
	if !ctx.Graph.IsBranch(ctx.Edge.From) {
		return out
	}
	for _, relation := range outcome.ReturnPresenceRelations {
		out = applyCallReturnPresenceRelation(ctx, facts, cache, resolver, branchRefinements, callPoint, targets, relation, out)
	}
	return out
}

func applyCallReturnPresenceRelation(
	ctx transfer.EdgeContext,
	facts factflow.Facts,
	cache *callOutcomeTraversalCache,
	resolver *visibility.Resolver,
	branchRefinements []factflow.BranchRefinement,
	callPoint cfg.Point,
	targets []factflow.CallResultTargetView,
	relation CallReturnPresenceRelation,
	out state.State,
) state.State {
	triggerTarget, ok := callOutcomeTargetForResult(targets, relation.TriggerIndex)
	if !ok || !callOutcomeRelatableTarget(triggerTarget) {
		return out
	}
	target, ok := callOutcomeTargetForResult(targets, relation.TargetIndex)
	if !ok || !callOutcomeRelatableTarget(target) {
		return out
	}
	triggerAssign, ok := callOutcomeResultAssignmentPoint(cache, ctx.Graph, facts, callPoint, triggerTarget, relation.TriggerIndex)
	if !ok {
		return out
	}
	targetAssign, ok := callOutcomeResultAssignmentPoint(cache, ctx.Graph, facts, callPoint, target, relation.TargetIndex)
	if !ok {
		return out
	}
	relationTargets := callOutcomePresenceTargets{
		callPoint:     callPoint,
		triggerTarget: triggerTarget,
		target:        target,
		triggerAssign: triggerAssign,
		targetAssign:  targetAssign,
		establish:     callOutcomeLaterPoint(cache, ctx.Graph, targetAssign, triggerAssign),
	}
	activeIn := callOutcomeRelationActiveIn(cache, ctx.Graph, facts, relationTargets)
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

func callOutcomeResultAssignmentPoint(
	cache *callOutcomeTraversalCache,
	graph cfg.Graph,
	facts factflow.Facts,
	callPoint cfg.Point,
	target factflow.CallResultTargetView,
	resultIndex int,
) (cfg.Point, bool) {
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	key := callOutcomeAssignmentKey{
		callPoint:   callPoint,
		resultIndex: resultIndex,
		targetIndex: target.Index(),
		targetPath:  target.TargetPathKey(),
	}
	if cache.assignmentPoints != nil {
		if point, ok := cache.assignmentPoints[key]; ok {
			return point, point != 0
		}
	} else {
		cache.assignmentPoints = make(map[callOutcomeAssignmentKey]cfg.Point)
	}
	for _, point := range cache.graphRPO(graph) {
		if assignment, ok := facts.RootAssignment(point); ok &&
			target.TargetPathEqual(assignment.TargetPath()) &&
			callOutcomeValueSourceConsumesResult(assignment.Source(), callPoint, target, resultIndex) {
			cache.assignmentPoints[key] = point
			return point, true
		}
	}
	cache.assignmentPoints[key] = 0
	return 0, false
}

func callOutcomeValueSourceConsumesResult(
	source factflow.ValueSource,
	callPoint cfg.Point,
	target factflow.CallResultTargetView,
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

func callOutcomeLaterPoint(cache *callOutcomeTraversalCache, graph cfg.Graph, first, second cfg.Point) cfg.Point {
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	order := cache.graphPointOrder(graph)
	if order[second] > order[first] {
		return second
	}
	return first
}

func callOutcomeRelationActiveIn(
	cache *callOutcomeTraversalCache,
	graph cfg.Graph,
	facts factflow.Facts,
	targets callOutcomePresenceTargets,
) map[cfg.Point]bool {
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	key := callOutcomeActiveInKey{
		callPoint:     targets.callPoint,
		triggerIndex:  targets.triggerTarget.Index(),
		targetIndex:   targets.target.Index(),
		triggerAssign: targets.triggerAssign,
		targetAssign:  targets.targetAssign,
		triggerPath:   targets.triggerTarget.TargetPathKey(),
		targetPath:    targets.target.TargetPathKey(),
	}
	if cache.activeIn != nil {
		if activeIn, ok := cache.activeIn[key]; ok {
			return activeIn
		}
	} else {
		cache.activeIn = make(map[callOutcomeActiveInKey]map[cfg.Point]bool)
	}
	rpo := cache.graphRPO(graph)
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
	cache.activeIn[key] = activeIn
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
	return targets.target.TargetPathEqual(targetPath) || targets.triggerTarget.TargetPathEqual(targetPath)
}

func callOutcomeBranchRefinesPath(
	branchRefinements []factflow.BranchRefinement,
	target factflow.CallResultTargetView,
) bool {
	for _, refinement := range branchRefinements {
		if pathsMatchForBranchRelation(refinement.TargetPath(), target.TargetPath()) {
			return true
		}
	}
	return false
}

func pathsMatchForBranchRelation(left, right pathdom.Path) bool {
	if left.Symbol != 0 || right.Symbol != 0 {
		if left.Symbol != right.Symbol {
			return false
		}
		if left.Version != 0 && right.Version != 0 && left.Version != right.Version {
			return false
		}
	} else if left.Root != right.Root {
		return false
	}
	if len(left.Segments) != len(right.Segments) {
		return false
	}
	for i := range left.Segments {
		lseg, rseg := left.Segments[i], right.Segments[i]
		if lseg.Kind != rseg.Kind || lseg.Name != rseg.Name || lseg.Index != rseg.Index {
			return false
		}
	}
	return true
}
