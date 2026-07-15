package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

func projectEffectDeltasBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.effectDeltas.top {
		out.effectDeltas = source.effectDeltas
		return true
	}
	values := projectFiniteMap(source.effectDeltas.values, func(key effectdelta.Key, _ effectdelta.Value) bool { return ctx.closure.ContainsPath(key.Target) })
	for key, value := range values {
		value.Before = product.ProjectBoundary(ctx.reg, value.Before)
		value.After = product.ProjectBoundary(ctx.reg, value.After)
		values[key] = value
	}
	out.effectDeltas = effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values)
	return true
}
func rebaseEffectDeltasBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.effectDeltas.top {
		out.effectDeltas = source.effectDeltas
		return true
	}
	values := make(map[effectdelta.Key]effectdelta.Value, len(source.effectDeltas.values))
	for key, value := range source.effectDeltas.values {
		paths, ok := boundaryRebasePaths(ctx, key.Target)
		if !ok {
			return false
		}
		value.Before, ok = rebaseBoundaryProduct(ctx, value.Before)
		if !ok {
			return false
		}
		value.After, ok = rebaseBoundaryProduct(ctx, value.After)
		if !ok {
			return false
		}
		for _, path := range paths {
			nextKey := key
			nextKey.Target = path
			candidate := value
			if existing, exists := values[nextKey]; exists {
				candidate = effectdelta.Domain(ctx.reg).Join(existing, candidate)
			}
			values[nextKey] = candidate
		}
	}
	out.effectDeltas = effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values)
	return true
}
func applyEffectDeltasBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.effectDeltas.top || fragment.effectDeltas.top {
		out.effectDeltas = effectDeltaLane{mapLane: mapLane[effectdelta.Key, effectdelta.Value]{top: true}}
		return true
	}
	values := applyFiniteMap(destination.effectDeltas.values, fragment.effectDeltas.values, func(key effectdelta.Key, _ effectdelta.Value) bool { return ctx.closure.ContainsPath(key.Target) })
	out.effectDeltas = effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values)
	return true
}
func equalEffectDeltasBoundary(reg *axis.Registry, a, b State) bool {
	d := effectdelta.MapDomain(reg)
	return d.Equal(a.effectDeltas.asMap(d), b.effectDeltas.asMap(d))
}
