package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func branchPresenceRelationRefinement(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) (factflow.ValueRefinement, bool) {
	triggerPath := relation.TriggerPathRef()
	for _, branchRefinement := range branchRefinements {
		if !pathsMatchForBranchRelation(branchRefinement.TargetPathRef(), triggerPath) {
			continue
		}
		refinement, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond)
		if !ok || !refinement.HasPresence(relation.TriggerPresence()) {
			continue
		}
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	if branchEdgeImpliesAbsentFromNonFalseFalsy(typeValues, ctx, resolver, projectPath, out, branchRefinements, relation) {
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
}

func branchEdgeImpliesAbsentFromNonFalseFalsy(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) bool {
	if !presence.Equal(relation.TriggerPresence(), presence.Absent()) {
		return false
	}
	triggerPath := relation.TriggerPathRef()
	for _, branchRefinement := range branchRefinements {
		if !pathsMatchForBranchRelation(branchRefinement.TargetPathRef(), triggerPath) {
			continue
		}
		if _, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond); ok {
			continue
		}
		opposite, ok := branchRefinement.ValueForEdge(!ctx.Edge.Cond)
		if !ok || !opposite.HasPresence(presence.Present()) {
			continue
		}
		if branchTriggerCanBeFalse(typeValues, ctx, resolver, projectPath, out, triggerPath) {
			continue
		}
		return true
	}
	return false
}

func branchTriggerCanBeFalse(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	triggerPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Edge.From, out, triggerPath, projectPath)
	if !ok {
		return true
	}
	kinds := product.Get(ctx.Registry, current.value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return true
	}
	return kinds.Contains(runtimekind.Boolean)
}

func presenceRefinement(reg *axis.Registry, value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, value))
}
