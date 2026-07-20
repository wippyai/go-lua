package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func boundaryTypestateResource(keys *keyspace.KeySpace, closure BoundaryClosure, resource typestate.Resource) bool {
	key, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(resource.ID.String()))
	return ok && boundaryContainsStateKey(keys, closure, key)
}
func projectTypestatesBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.typestates, _ = projectTypestatesBoundaryFactor(ctx, source.typestates)
	return true
}
func projectTypestatesBoundaryFactor(ctx *boundaryProjectContext, source typestate.Store) (typestate.Store, bool) {
	if typestate.Equal(source, typestate.Domain.Top()) {
		return typestate.Domain.Top(), true
	}
	resources := source.Resources()
	kept := resources[:0]
	for _, resource := range resources {
		if boundaryTypestateResource(ctx.keys, ctx.closure, resource) {
			kept = append(kept, resource)
		}
	}
	return source.Restrict(kept), true
}
func rebaseTypestatesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.typestates, ok = rebaseTypestatesBoundaryFactor(ctx, source.typestates)
	return ok
}
func rebaseTypestatesBoundaryFactor(ctx *boundaryRebaseContext, source typestate.Store) (typestate.Store, bool) {
	if typestate.Equal(source, typestate.Domain.Top()) {
		return typestate.Domain.Top(), true
	}
	result := typestate.Domain.Bottom()
	for _, resource := range source.Resources() {
		key, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(resource.ID.String()))
		if !ok {
			return typestate.Store{}, false
		}
		next, ok := rebaseBoundaryStateKeys(ctx, key)
		if !ok {
			return typestate.Store{}, false
		}
		one := source.Restrict([]typestate.Resource{resource})
		for _, target := range next {
			mapped := one.MapResources(func(current typestate.Resource) typestate.Resource {
				current.ID = typestate.ResourceID(target.String())
				return current
			})
			result = typestate.Domain.Join(result, mapped)
		}
	}
	return result, true
}
func applyTypestatesBoundaryLane(ctx *boundaryApplyContext, destination, fragment typestate.Store) (typestate.Store, bool) {
	if typestate.Equal(destination, typestate.Domain.Top()) || typestate.Equal(fragment, typestate.Domain.Top()) {
		return typestate.Domain.Top(), true
	}
	resources := destination.Resources()
	outside := resources[:0]
	for _, resource := range resources {
		if !boundaryTypestateResource(ctx.keys, ctx.closure, resource) {
			outside = append(outside, resource)
		}
	}
	return destination.Restrict(outside).Overlay(fragment), true
}
func equalTypestatesBoundary(_ *axis.Registry, a, b State) bool {
	return typestate.Equal(a.typestates, b.typestates)
}
