package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type pathValue struct {
	value product.Value
	write func(state.State, product.Value) state.State
}

func applyBranchPathRelation(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	relation factflow.BranchPathRelation,
) state.State {
	switch relation.Kind() {
	case factflow.BranchPathRelationEqual:
		return applyBranchPathEquality(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath())
	case factflow.BranchPathRelationNotEqual:
		return applyBranchPathInequality(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath())
	default:
		return out
	}
}

func applyBranchPathEquality(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if selected, ok := applyChannelSelectCaseEquality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath); ok {
		return selected
	}
	return applyPathEqualityAtCached(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath)
}

func applyPathEqualityAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	return applyPathEqualityAtCached(nil, reg, resolver, projectPath, point, out, leftPath, rightPath)
}

func applyPathEqualityAtCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	left, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, leftPath, projectPath)
	if !ok {
		return out
	}
	right, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, rightPath, projectPath)
	if !ok {
		return out
	}
	meet := product.Meet(reg, left.value, right.value)
	out = left.write(out, meet)
	out = right.write(out, meet)
	out = applyPathOriginRelation(reg, resolver, projectPath, point, out, leftPath, rightPath, true)
	out = applyPathOriginRelation(reg, resolver, projectPath, point, out, rightPath, leftPath, true)
	return out
}

func applyBranchPathInequality(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if selected, ok := applyChannelSelectCaseInequality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath); ok {
		return selected
	}
	out = applyPathOriginRelation(ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath, false)
	out = applyPathOriginRelation(ctx.Registry, resolver, projectPath, ctx.Edge.From, out, rightPath, leftPath, false)
	return out
}

func resolvePathValueAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (pathValue, bool) {
	return resolvePathValueAtCached(nil, reg, resolver, point, out, targetPath, projectPath)
}

func resolvePathValueAtCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
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
		if ok {
			value = projected
		} else if projected, ok := projectPathStructuralValueCached(typeValues, reg, out, targetPath, projectPath); ok {
			value = projected
		} else {
			return pathValue{}, false
		}
	}
	return pathValue{
		value: value,
		write: func(s state.State, value product.Value) state.State {
			return s.WritePathKey(reg, pathKey, value)
		},
	}, true
}

func projectPathStructuralValue(reg *axis.Registry, out state.State, targetPath pathdom.Path, projectPath PathTypeProjector) (product.Value, bool) {
	return projectPathStructuralValueCached(nil, reg, out, targetPath, projectPath)
}

func projectPathStructuralValueCached(typeValues *typevalue.Cache, reg *axis.Registry, out state.State, targetPath pathdom.Path, projectPath PathTypeProjector) (product.Value, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	if projectPath == nil {
		return product.Value{}, false
	}
	root := out.ReadValue(reg, key.SymbolValue(targetPath.Symbol))
	if product.Equal(reg, root, product.Bottom(reg)) {
		return product.Value{}, false
	}
	rootType, ok := structuralTypeFromPathValueCached(typeValues, reg, root)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := projectPath(rootType, targetPath)
	if !ok {
		return product.Value{}, false
	}
	return typeValues.FromTypeWithWitness(reg, projected), true
}

func structuralTypeFromPathValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return structuralTypeFromPathValueCached(nil, reg, value)
}

func structuralTypeFromPathValueCached(typeValues *typevalue.Cache, reg *axis.Registry, value product.Value) (typ.Type, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	valuePresence := product.PresenceOf(value)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			t = typeForPathValuePresence(t, valuePresence)
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := typeValues.NarrowVariantByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
				if narrowed, ok := typeValues.TypeFromVariantOrigin(origin.Family(), origin.Cases()); ok {
					return typeForPathValuePresence(narrowed, valuePresence), true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := typeValues.TypeFromVariantOrigin(origin.Family(), origin.Cases()); ok {
			return typeForPathValuePresence(t, valuePresence), true
		}
	}
	return nil, false
}

func typeForPathValuePresence(t typ.Type, p presence.Value) typ.Type {
	switch {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Present()):
		if present := table.PresentReadonlyEntryValue(t); present != nil {
			return present
		}
	}
	return t
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
	family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.Cases(), targetPath.Segments)
	if !ok {
		return product.Value{}, false
	}
	return product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(family, cases)), true
}

func applyPathOriginRelation(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	parentPath pathdom.Path,
	constraintPath pathdom.Path,
	equal bool,
) state.State {
	if parentPath.Symbol == 0 || len(parentPath.Segments) == 0 {
		return out
	}
	constraint, ok := resolvePathValueAt(reg, resolver, point, out, constraintPath, projectPath)
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
	cases, ok := variant.NarrowOriginByPath(
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
