package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func projectHeapBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.heapTableIdentity.top {
		out.heapTableIdentity = source.heapTableIdentity
		return true
	}
	values := projectFiniteMap(source.heapTableIdentity.values, func(id identity.ID, _ heapidentity.TableObject) bool { return ctx.closure.ContainsIdentity(id) })
	for id, object := range values {
		values[id] = object.MapValues(ctx.reg, func(value product.Value) product.Value {
			return product.ProjectBoundary(ctx.reg, value)
		})
	}
	out.heapTableIdentity = heapTableIdentityLaneFromMap(heapidentity.MapDomain(ctx.reg), values)
	return true
}
func rebaseHeapBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.heapTableIdentity.top {
		out.heapTableIdentity = source.heapTableIdentity
		return true
	}
	values := make(map[identity.ID]heapidentity.TableObject, len(source.heapTableIdentity.values))
	for id, object := range source.heapTableIdentity.values {
		nextID, ok := RebaseBoundaryIdentity(ctx.allocations, id)
		if !ok {
			return false
		}
		object, ok = object.Rekey(ctx.fromKeys, ctx.toKeys)
		if !ok {
			return false
		}
		valid := true
		object = object.MapValues(ctx.reg, func(value product.Value) product.Value {
			next, good := rebaseBoundaryProduct(ctx, value)
			valid = valid && good
			return next
		})
		if !valid {
			return false
		}
		values[nextID] = object
	}
	out.heapTableIdentity = heapTableIdentityLaneFromMap(heapidentity.MapDomain(ctx.reg), values)
	return true
}
func applyHeapBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.heapTableIdentity.top || fragment.heapTableIdentity.top {
		out.heapTableIdentity = heapTableIdentityLane{top: true}
		return true
	}
	values := applyFiniteMap(destination.heapTableIdentity.values, fragment.heapTableIdentity.values, func(id identity.ID, _ heapidentity.TableObject) bool { return ctx.closure.ContainsIdentity(id) })
	out.heapTableIdentity = heapTableIdentityLaneFromMap(heapidentity.MapDomain(ctx.reg), values)
	return true
}
func equalHeapBoundary(reg *axis.Registry, a, b State) bool {
	d := heapidentity.MapDomain(reg)
	return d.Equal(a.heapTableIdentity.asMap(d), b.heapTableIdentity.asMap(d))
}
