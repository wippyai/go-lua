package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
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
		if _, ok := refinement.Constraint(); ok {
			out = invalidateRootDescendantsAt(resolver, point, out, targetPath)
		}
		return out.UpdateValue(reg, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(reg, value, refinement)
		})
	}
	if resolver == nil {
		return out
	}
	current, ok := resolvePathValueAt(reg, resolver, point, out, targetPath)
	if !ok {
		constraint, hasConstraint := refinement.Constraint()
		if !hasConstraint {
			return out
		}
		written, wrote := writePathAt(reg, out, resolver, point, targetPath, constraint)
		if !wrote {
			return out
		}
		return written
	}
	return current.write(out, refineProductValue(reg, current.value, refinement))
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
	resolver *visibility.Resolver,
	out state.State,
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
	if branchEdgeImpliesAbsentFromNonFalseFalsy(ctx, resolver, out, branchRefinements, relation) {
		return presenceRefinement(ctx.Registry, relation.TargetPresence()), true
	}
	return factflow.ValueRefinement{}, false
}

func branchEdgeImpliesAbsentFromNonFalseFalsy(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	branchRefinements []factflow.BranchRefinement,
	relation factflow.BranchPresenceRelation,
) bool {
	if !presence.Equal(relation.TriggerPresence(), presence.Absent()) {
		return false
	}
	triggerPath := relation.TriggerPath()
	for _, branchRefinement := range branchRefinements {
		if !branchRefinement.TargetPath().Equal(triggerPath) {
			continue
		}
		if _, ok := branchRefinement.ValueForEdge(ctx.Edge.Cond); ok {
			continue
		}
		opposite, ok := branchRefinement.ValueForEdge(!ctx.Edge.Cond)
		if !ok || !refinementHasPresence(opposite, presence.Present()) {
			continue
		}
		if branchTriggerCanBeFalse(ctx, resolver, out, triggerPath) {
			continue
		}
		return true
	}
	return false
}

func branchTriggerCanBeFalse(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	triggerPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Edge.From, out, triggerPath)
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
	case factflow.BranchPathRelationNotEqual:
		return applyBranchPathInequality(ctx, resolver, out, relation.LeftPath(), relation.RightPath())
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
	out = applyPathOriginRelation(reg, resolver, point, out, leftPath, rightPath, true)
	out = applyPathOriginRelation(reg, resolver, point, out, rightPath, leftPath, true)
	return out
}

func applyBranchPathInequality(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	out = applyPathOriginRelation(ctx.Registry, resolver, ctx.Edge.From, out, leftPath, rightPath, false)
	out = applyPathOriginRelation(ctx.Registry, resolver, ctx.Edge.From, out, rightPath, leftPath, false)
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
	value := out.ReadPathKey(reg, pathKey)
	if product.Equal(reg, value, product.Bottom(reg)) {
		projected, ok := projectPathOriginValue(reg, out, targetPath)
		if !ok {
			return pathValue{}, false
		}
		value = projected
	}
	return pathValue{
		value: value,
		write: func(s state.State, value product.Value) state.State {
			return s.WritePathKey(reg, pathKey, value)
		},
	}, true
}

func projectPathOriginValue(reg *axis.Registry, out state.State, targetPath pathdom.Path) (product.Value, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	root := out.ReadValue(reg, key.SymbolValue(targetPath.Symbol))
	if product.Equal(reg, root, product.Bottom(reg)) {
		return product.Value{}, false
	}
	origin := product.Get(reg, root, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return product.Value{}, false
	}
	family, cases, ok := discriminant.ProjectOrigin(origin.Family(), origin.Cases(), targetPath.Segments)
	if !ok {
		return product.Value{}, false
	}
	return product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(family, cases)), true
}

func applyPathOriginRelation(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	parentPath pathdom.Path,
	constraintPath pathdom.Path,
	equal bool,
) state.State {
	if parentPath.Symbol == 0 || len(parentPath.Segments) == 0 {
		return out
	}
	constraint, ok := resolvePathValueAt(reg, resolver, point, out, constraintPath)
	if !ok {
		return out
	}
	constraintOrigin := product.Get(reg, constraint.value, variantorigin.Key)
	if constraintOrigin.IsBottom() || constraintOrigin.IsTop() {
		return out
	}
	slot := key.SymbolValue(parentPath.Symbol)
	root := out.ReadValue(reg, slot)
	if product.Equal(reg, root, product.Bottom(reg)) {
		return out
	}
	rootOrigin := product.Get(reg, root, variantorigin.Key)
	if rootOrigin.IsBottom() || rootOrigin.IsTop() {
		return out
	}
	cases, ok := discriminant.NarrowOriginByPath(
		rootOrigin.Family(),
		rootOrigin.Cases(),
		parentPath.Segments,
		constraintOrigin.Family(),
		constraintOrigin.Cases(),
		equal,
	)
	if !ok {
		return out
	}
	narrowed := rootOrigin
	if len(cases) == 0 {
		narrowed = variantorigin.Bottom()
	} else {
		narrowed = variantorigin.Of(rootOrigin.Family(), cases)
	}
	rootPath := parentPath
	rootPath.Segments = nil
	out = invalidateRootDescendantsAt(resolver, point, out, rootPath)
	return out.WriteValue(reg, slot, product.Set(reg, root, variantorigin.Key, narrowed))
}

func invalidateRootDescendantsAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	rootPath pathdom.Path,
) state.State {
	if resolver == nil || rootPath.Symbol == 0 || len(rootPath.Segments) != 0 {
		return out
	}
	invalidated, ok := invalidatePathDescendantsAt(out, resolver, point, rootPath)
	if !ok {
		return out
	}
	return invalidated
}
