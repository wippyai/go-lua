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
	if typestate.Equal(source.typestates, typestate.Domain.Top()) {
		out.typestates = typestate.Domain.Top()
		return true
	}
	resources := source.typestates.Resources()
	kept := resources[:0]
	for _, resource := range resources {
		if boundaryTypestateResource(ctx.keys, ctx.closure, resource) {
			kept = append(kept, resource)
		}
	}
	out.typestates = source.typestates.Restrict(kept)
	return true
}
func rebaseTypestatesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if typestate.Equal(source.typestates, typestate.Domain.Top()) {
		out.typestates = typestate.Domain.Top()
		return true
	}
	result := typestate.Domain.Bottom()
	for _, resource := range source.typestates.Resources() {
		key, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(resource.ID.String()))
		if !ok {
			return false
		}
		next, ok := rebaseBoundaryStateKeys(ctx, key)
		if !ok {
			return false
		}
		one := source.typestates.Restrict([]typestate.Resource{resource})
		for _, target := range next {
			mapped := one.MapResources(func(current typestate.Resource) typestate.Resource {
				current.ID = typestate.ResourceID(target.String())
				return current
			})
			result = typestate.Domain.Join(result, mapped)
		}
	}
	out.typestates = result
	return true
}
func applyTypestatesBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if typestate.Equal(destination.typestates, typestate.Domain.Top()) || typestate.Equal(fragment.typestates, typestate.Domain.Top()) {
		out.typestates = typestate.Domain.Top()
		return true
	}
	resources := destination.typestates.Resources()
	outside := resources[:0]
	for _, resource := range resources {
		if !boundaryTypestateResource(ctx.keys, ctx.closure, resource) {
			outside = append(outside, resource)
		}
	}
	out.typestates = destination.typestates.Restrict(outside).Overlay(fragment.typestates)
	return true
}
func equalTypestatesBoundary(_ *axis.Registry, a, b State) bool {
	return typestate.Equal(a.typestates, b.typestates)
}
