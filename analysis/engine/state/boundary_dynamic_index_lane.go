package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func projectDynamicIndexBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.dynamicIndex.top {
		out.dynamicIndex = source.dynamicIndex
		return true
	}
	values := projectFiniteMap(source.dynamicIndex.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool { return ctx.closure.ContainsPath(key.Table) })
	for key, value := range values {
		value.KeyValue = product.ProjectBoundary(ctx.reg, value.KeyValue)
		value.Value = product.ProjectBoundary(ctx.reg, value.Value)
		values[key] = value
	}
	out.dynamicIndex = dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values)
	return true
}
func rebaseDynamicIndexBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.dynamicIndex.top {
		out.dynamicIndex = source.dynamicIndex
		return true
	}
	values := make(map[dynamicindex.Key]dynamicindex.Fact, len(source.dynamicIndex.values))
	for key, value := range source.dynamicIndex.values {
		paths, ok := boundaryRebasePaths(ctx, key.Table)
		if !ok {
			return false
		}
		value.KeyValue, ok = rebaseBoundaryProduct(ctx, value.KeyValue)
		if !ok {
			return false
		}
		value.Value, ok = rebaseBoundaryProduct(ctx, value.Value)
		if !ok {
			return false
		}
		for _, path := range paths {
			nextKey := key
			nextKey.Table = path
			candidate := value
			if existing, exists := values[nextKey]; exists {
				candidate = dynamicindex.Domain(ctx.reg).Join(existing, candidate)
			}
			values[nextKey] = candidate
		}
	}
	out.dynamicIndex = dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values)
	return true
}
func applyDynamicIndexBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.dynamicIndex.top || fragment.dynamicIndex.top {
		out.dynamicIndex = dynamicIndexLane{mapLane: mapLane[dynamicindex.Key, dynamicindex.Fact]{top: true}}
		return true
	}
	values := applyFiniteMap(destination.dynamicIndex.values, fragment.dynamicIndex.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool { return ctx.closure.ContainsPath(key.Table) })
	out.dynamicIndex = dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values)
	return true
}
func equalDynamicIndexBoundary(reg *axis.Registry, a, b State) bool {
	d := dynamicindex.MapDomain(reg)
	return d.Equal(a.dynamicIndex.asMap(d), b.dynamicIndex.asMap(d))
}
