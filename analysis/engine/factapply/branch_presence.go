package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func branchPresenceRelationRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) (factflow.ValueRefinement, bool) {
	triggerPath := relation.TriggerPath()
	for _, branchRefinement := range branchRefinements {
		if !pathsMatchForBranchRelation(branchRefinement.TargetPath(), triggerPath) {
			continue
		}
		refinement, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond)
		if !ok || !refinementHasPresence(refinement, relation.TriggerPresence()) {
			continue
		}
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	if branchEdgeImpliesAbsentFromNonFalseFalsy(ctx, resolver, projectPath, out, branchRefinements, relation) {
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
}

func branchEdgeImpliesAbsentFromNonFalseFalsy(
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
	triggerPath := relation.TriggerPath()
	for _, branchRefinement := range branchRefinements {
		if !pathsMatchForBranchRelation(branchRefinement.TargetPath(), triggerPath) {
			continue
		}
		if _, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond); ok {
			continue
		}
		opposite, ok := branchRefinement.ValueForEdge(!ctx.Edge.Cond)
		if !ok || !refinementHasPresence(opposite, presence.Present()) {
			continue
		}
		if branchTriggerCanBeFalse(ctx, resolver, projectPath, out, triggerPath) {
			continue
		}
		return true
	}
	return false
}

func branchTriggerCanBeFalse(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	triggerPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Edge.From, out, triggerPath, projectPath)
	if !ok {
		return true
	}
	kinds := product.Get(ctx.Registry, current.value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return true
	}
	return kinds.Contains(runtimekind.Boolean)
}

func refinementHasPresence(refinement factflow.ValueRefinement, want presence.Value) bool {
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	return presence.Equal(product.PresenceOf(constraint), want)
}

func presenceRefinement(reg *axis.Registry, value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, value))
}
