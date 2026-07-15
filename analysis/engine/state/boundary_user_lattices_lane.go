package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

func projectUserLatticesBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.userLattices.top {
		out.userLattices = source.userLattices
		return true
	}
	out.userLattices.values = projectFiniteMap(source.userLattices.values, func(key userLatticeKey, _ userlattice.Element) bool { return ctx.closure.ContainsPath(key.path) })
	return true
}
func rebaseUserLatticesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.userLattices.top {
		out.userLattices = source.userLattices
		return true
	}
	values := make(map[userLatticeKey]userlattice.Element, len(source.userLattices.values))
	runtime := userlattice.RuntimeFor(ctx.reg)
	for key, value := range source.userLattices.values {
		paths, ok := boundaryRebasePaths(ctx, key.path)
		if !ok {
			return false
		}
		axis, ok := runtime.AxisBySlot(key.axis)
		if !ok {
			return false
		}
		for _, path := range paths {
			nextKey := key
			nextKey.path = path
			candidate := value
			if existing, exists := values[nextKey]; exists {
				candidate = axis.Join(existing, candidate)
			}
			values[nextKey] = candidate
		}
	}
	out.userLattices.values = values
	return true
}
func applyUserLatticesBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.userLattices.top || fragment.userLattices.top {
		out.userLattices = userLatticeLane{top: true}
		return true
	}
	out.userLattices.values = applyFiniteMap(destination.userLattices.values, fragment.userLattices.values, func(key userLatticeKey, _ userlattice.Element) bool { return ctx.closure.ContainsPath(key.path) })
	return true
}
func equalUserLatticesBoundary(reg *axis.Registry, a, b State) bool {
	return userLatticeDomain(userlattice.RuntimeFor(reg)).Equal(a.userLattices, b.userLattices)
}
