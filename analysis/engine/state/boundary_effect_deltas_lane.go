package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

func projectEffectDeltasBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.effectDeltas, _ = projectEffectDeltasBoundaryFactor(ctx, source.effectDeltas)
	return true
}
func projectEffectDeltasBoundaryFactor(ctx *boundaryProjectContext, source effectDeltaLane) (effectDeltaLane, bool) {
	if source.top {
		return source, true
	}
	values := projectFiniteMap(source.values, func(key effectdelta.Key, _ effectdelta.Value) bool { return ctx.closure.ContainsPath(key.Target) })
	for key, value := range values {
		value.Before = product.ProjectBoundary(ctx.reg, value.Before)
		value.After = product.ProjectBoundary(ctx.reg, value.After)
		values[key] = value
	}
	return effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values), true
}
func rebaseEffectDeltasBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.effectDeltas, ok = rebaseEffectDeltasBoundaryFactor(ctx, source.effectDeltas)
	return ok
}
func rebaseEffectDeltasBoundaryFactor(ctx *boundaryRebaseContext, source effectDeltaLane) (effectDeltaLane, bool) {
	if source.top {
		return source, true
	}
	values := make(map[effectdelta.Key]effectdelta.Value, len(source.values))
	for key, value := range source.values {
		paths, ok := boundaryRebasePaths(ctx, key.Target)
		if !ok {
			return effectDeltaLane{}, false
		}
		value.Before, ok = rebaseBoundaryProduct(ctx, value.Before)
		if !ok {
			return effectDeltaLane{}, false
		}
		value.After, ok = rebaseBoundaryProduct(ctx, value.After)
		if !ok {
			return effectDeltaLane{}, false
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
	return effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values), true
}
func applyEffectDeltasBoundaryLane(ctx *boundaryApplyContext, destination, fragment effectDeltaLane) (effectDeltaLane, bool) {
	if destination.top || fragment.top {
		return effectDeltaLane{mapLane: mapLane[effectdelta.Key, effectdelta.Value]{top: true}}, true
	}
	values := applyFiniteMap(destination.values, fragment.values, func(key effectdelta.Key, _ effectdelta.Value) bool { return ctx.closure.ContainsPath(key.Target) })
	return effectDeltaLaneFromMap(effectdelta.MapDomain(ctx.reg), values), true
}
func equalEffectDeltasBoundary(reg *axis.Registry, a, b State) bool {
	d := effectdelta.MapDomain(reg)
	return d.Equal(a.effectDeltas.asMap(d), b.effectDeltas.asMap(d))
}
