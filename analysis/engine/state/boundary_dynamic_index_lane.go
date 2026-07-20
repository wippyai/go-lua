package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func projectDynamicIndexBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.dynamicIndex, _ = projectDynamicIndexBoundaryFactor(ctx, source.dynamicIndex)
	return true
}
func projectDynamicIndexBoundaryFactor(ctx *boundaryProjectContext, source dynamicIndexLane) (dynamicIndexLane, bool) {
	if source.top {
		return source, true
	}
	values := projectFiniteMap(source.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool { return ctx.closure.ContainsPath(key.Table) })
	for key, value := range values {
		value.KeyValue = product.ProjectBoundary(ctx.reg, value.KeyValue)
		value.Value = product.ProjectBoundary(ctx.reg, value.Value)
		values[key] = value
	}
	return dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values), true
}
func rebaseDynamicIndexBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.dynamicIndex, ok = rebaseDynamicIndexBoundaryFactor(ctx, source.dynamicIndex)
	return ok
}
func rebaseDynamicIndexBoundaryFactor(ctx *boundaryRebaseContext, source dynamicIndexLane) (dynamicIndexLane, bool) {
	if source.top {
		return source, true
	}
	values := make(map[dynamicindex.Key]dynamicindex.Fact, len(source.values))
	for key, value := range source.values {
		paths, ok := boundaryRebasePaths(ctx, key.Table)
		if !ok {
			return dynamicIndexLane{}, false
		}
		value.KeyValue, ok = rebaseBoundaryProduct(ctx, value.KeyValue)
		if !ok {
			return dynamicIndexLane{}, false
		}
		value.Value, ok = rebaseBoundaryProduct(ctx, value.Value)
		if !ok {
			return dynamicIndexLane{}, false
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
	return dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values), true
}
func applyDynamicIndexBoundaryLane(ctx *boundaryApplyContext, destination, fragment dynamicIndexLane) (dynamicIndexLane, bool) {
	if destination.top || fragment.top {
		return dynamicIndexLane{mapLane: mapLane[dynamicindex.Key, dynamicindex.Fact]{top: true}}, true
	}
	values := applyFiniteMap(destination.values, fragment.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool { return ctx.closure.ContainsPath(key.Table) })
	return dynamicIndexLaneFromMap(dynamicindex.MapDomain(ctx.reg), values), true
}
func equalDynamicIndexBoundary(reg *axis.Registry, a, b State) bool {
	d := dynamicindex.MapDomain(reg)
	return d.Equal(a.dynamicIndex.asMap(d), b.dynamicIndex.asMap(d))
}
