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
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyBranchRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	return applyValueRefinementAt(ctx.Registry, resolver, ctx.Edge.From, out, targetPath, refinement)
}

func applyValueRefinementAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 {
		return out
	}
	if len(targetPath.Segments) == 0 {
		return out.UpdateValue(reg, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(reg, value, refinement)
		})
	}
	if resolver == nil {
		return out
	}
	updated, ok := updatePathAt(reg, out, resolver, point, targetPath, func(value product.Value) product.Value {
		return refineProductValue(reg, value, refinement)
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

type pathValue struct {
	value product.Value
	write func(state.State, product.Value) state.State
}

func applyBranchPathRelation(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	relation factflow.BranchPathRelation,
) state.State {
	switch relation.Kind() {
	case factflow.BranchPathRelationEqual:
		return applyBranchPathEquality(ctx, resolver, out, relation.LeftPath(), relation.RightPath())
	default:
		return out
	}
}

func applyBranchPathEquality(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	return applyPathEqualityAt(ctx.Registry, resolver, ctx.Edge.From, out, leftPath, rightPath)
}

func applyPathEqualityAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	left, ok := resolvePathValueAt(reg, resolver, point, out, leftPath)
	if !ok {
		return out
	}
	right, ok := resolvePathValueAt(reg, resolver, point, out, rightPath)
	if !ok {
		return out
	}
	meet := product.Meet(reg, left.value, right.value)
	out = left.write(out, meet)
	out = right.write(out, meet)
	return out
}

func resolvePathValueAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) (pathValue, bool) {
	if targetPath.Symbol == 0 {
		return pathValue{}, false
	}
	if len(targetPath.Segments) == 0 {
		slot := key.SymbolValue(targetPath.Symbol)
		return pathValue{
			value: out.ReadValue(reg, slot),
			write: func(s state.State, value product.Value) state.State {
				return s.WriteValue(reg, slot, value)
			},
		}, true
	}
	if resolver == nil {
		return pathValue{}, false
	}
	pathKey := resolver.KeyAt(point, targetPath)
	if pathKey == "" {
		return pathValue{}, false
	}
	return pathValue{
		value: out.ReadPathKey(reg, pathKey),
		write: func(s state.State, value product.Value) state.State {
			return s.WritePathKey(reg, pathKey, value)
		},
	}, true
}
