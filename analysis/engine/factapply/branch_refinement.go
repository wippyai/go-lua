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
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	out = applyDescendantTruthyRootOriginRefinement(reg, resolver, point, out, targetPath, refinement)
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

func applyDescendantTruthyRootOriginRefinement(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return out
	}
	if !refinementHasPresentConstraint(refinement) {
		return out
	}
	rootSlot := key.SymbolValue(targetPath.Symbol)
	rootValue := out.ReadValue(reg, rootSlot)
	if product.Equal(reg, rootValue, product.Bottom(reg)) {
		return out
	}
	rootType, ok := structuralTypeFromRootValue(reg, rootValue)
	if !ok {
		return out
	}
	family, cases, ok := discriminant.OriginByPathLiteral(rootType, targetPath.Segments, typ.LiteralBool(true))
	if !ok {
		return out
	}
	narrowedType, ok := discriminant.NarrowByOrigin(rootType, family, cases)
	if !ok {
		return out
	}
	constraint := typevalue.FromType(reg, narrowedType)
	constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
	rootPath := targetPath
	rootPath.Segments = nil
	out = invalidateRootDescendantsAt(resolver, point, out, rootPath)
	return out.WriteValue(reg, rootSlot, product.Meet(reg, rootValue, constraint))
}

func refinementHasPresentConstraint(refinement factflow.ValueRefinement) bool {
	constraint, ok := refinement.Constraint()
	return ok && presence.Equal(product.PresenceOf(constraint), presence.Present())
}

func structuralTypeFromRootValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := discriminant.NarrowByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		return discriminant.TypeFromOrigin(origin.Family(), origin.Cases())
	}
	return nil, false
}
