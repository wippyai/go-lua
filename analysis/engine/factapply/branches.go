package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func applyBranchRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 {
		return out
	}
	if len(targetPath.Segments) == 0 {
		return out.UpdateValue(ctx.Registry, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(ctx.Registry, value, refinement)
		})
	}
	if resolver == nil {
		return out
	}
	updated, ok := updatePathAt(ctx.Registry, out, resolver, ctx.Edge.From, targetPath, func(value product.Value) product.Value {
		return refineProductValue(ctx.Registry, value, refinement)
	})
	if !ok {
		return out
	}
	return updated
}

func refineProductValue(reg *axis.Registry, value product.Value, refinement factflow.ValueRefinement) product.Value {
	constraint, ok := refinement.Constraint()
	if !ok {
		return value
	}
	return product.Meet(reg, value, constraint)
}

func branchPresenceRelationRefinement(
	ctx transfer.EdgeContext,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) (factflow.ValueRefinement, bool) {
	triggerPath := relation.TriggerPath()
	for _, branchRefinement := range branchRefinements {
		if !branchRefinement.TargetPath().Equal(triggerPath) {
			continue
		}
		refinement, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond)
		if !ok || !refinementHasPresence(refinement, relation.TriggerPresence()) {
			continue
		}
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
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
