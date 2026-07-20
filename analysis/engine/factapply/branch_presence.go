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
	if refinement, ok := branchPresenceRelationStaticRefinement(ctx.Registry, ctx.Edge.Cond, branchRefinements, relation); ok {
		return refinement, true
	}
	if branchEdgeImpliesAbsentFromNonFalseFalsy(typeValues, ctx, resolver, projectPath, out, branchRefinements, relation) {
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
}

// branchPresenceRelationStaticRefinement is the representation-independent
// part of presence implication: the selected edge's frozen branch refinement
// itself proves the trigger presence. It deliberately takes no State. Both the
// concrete and formal programs lower this theorem to the same target
// refinement factor; only the exceptional non-false-falsy case needs a
// runtime trigger value.
func branchPresenceRelationStaticRefinement(
	reg *axis.Registry,
	cond bool,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) (factflow.ValueRefinement, bool) {
	triggerPath := relation.TriggerPathRef()
	for _, branchRefinement := range branchRefinements {
		if !pathsMatchForBranchRelation(branchRefinement.TargetPathRef(), triggerPath) {
			continue
		}
		refinement, ok := branchRefinement.ValueForEdge(cond)
		if !ok || !refinement.HasPresence(relation.TriggerPresence()) {
			continue
		}
		return presenceRefinement(reg, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
}

// branchPresenceRelationNeedsNonBooleanTrigger identifies the sole dynamic
// presence theorem. When the opposite edge proves the trigger present but the
// selected edge carries no explicit refinement, the selected edge proves
// absence exactly when the trigger cannot be Boolean false. Runtime execution
// supplies only that registered value query; the relation shape is frozen here.
func branchPresenceRelationNeedsNonBooleanTrigger(
	cond bool,
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
		if _, ok := branchRefinement.ValueForEdge(cond); ok {
			continue
		}
		opposite, ok := branchRefinement.ValueForEdge(!cond)
		if ok && opposite.HasPresence(presence.Present()) {
			return true
		}
	}
	return false
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
	return branchPresenceRelationNeedsNonBooleanTrigger(ctx.Edge.Cond, branchRefinements, relation) &&
		!branchTriggerCanBeFalse(typeValues, ctx, resolver, projectPath, out, relation.TriggerPathRef())
}

func branchTriggerCanBeFalse(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	triggerPath pathdom.Path,
) bool {
	current, ok := branchFeasibilityValue(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, triggerPath)
	if !ok {
		return true
	}
	kinds := product.Get(ctx.Registry, current, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return true
	}
	return kinds.Contains(runtimekind.Boolean)
}

func presenceRefinement(reg *axis.Registry, value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, value))
}
