package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
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
