package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

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

func presenceRefinement(reg *axis.Registry, value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, value))
}
