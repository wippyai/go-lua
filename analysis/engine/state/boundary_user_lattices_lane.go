package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

func projectUserLatticesBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.userLattices, _ = projectUserLatticesBoundaryFactor(ctx, source.userLattices)
	return true
}
func projectUserLatticesBoundaryFactor(ctx *boundaryProjectContext, source userLatticeLane) (userLatticeLane, bool) {
	if source.top {
		return source, true
	}
	out := userLatticeLane{}
	out.values = projectFiniteMap(source.values, func(key userLatticeKey, _ userlattice.Element) bool { return ctx.closure.ContainsPath(key.path) })
	return out, true
}
func rebaseUserLatticesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.userLattices, ok = rebaseUserLatticesBoundaryFactor(ctx, source.userLattices)
	return ok
}
func rebaseUserLatticesBoundaryFactor(ctx *boundaryRebaseContext, source userLatticeLane) (userLatticeLane, bool) {
	if source.top {
		return source, true
	}
	values := make(map[userLatticeKey]userlattice.Element, len(source.values))
	runtime := userlattice.RuntimeFor(ctx.reg)
	for key, value := range source.values {
		paths, ok := boundaryRebasePaths(ctx, key.path)
		if !ok {
			return userLatticeLane{}, false
		}
		axis, ok := runtime.AxisBySlot(key.axis)
		if !ok {
			return userLatticeLane{}, false
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
	return userLatticeLane{values: values}, true
}
func applyUserLatticesBoundaryLane(ctx *boundaryApplyContext, destination, fragment userLatticeLane) (userLatticeLane, bool) {
	if destination.top || fragment.top {
		return userLatticeLane{top: true}, true
	}
	destination.values = applyFiniteMap(destination.values, fragment.values, func(key userLatticeKey, _ userlattice.Element) bool { return ctx.closure.ContainsPath(key.path) })
	return destination, true
}
func equalUserLatticesBoundary(reg *axis.Registry, a, b State) bool {
	return userLatticeDomain(userlattice.RuntimeFor(reg)).Equal(a.userLattices, b.userLattices)
}
