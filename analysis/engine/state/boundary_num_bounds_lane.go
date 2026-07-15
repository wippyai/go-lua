package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
)

func projectNumBoundBoundary(ctx *boundaryProjectContext, lane numBoundLane) numBoundLane {
	if lane.lane.Bottom() {
		return lane
	}
	return numBoundLane{lane: lift.MustMapValues(projectFiniteMap(lane.lane.Values(), func(path keyspace.Key, _ int64) bool { return ctx.closure.ContainsPath(path) }))}
}
func rebaseNumBoundBoundary(ctx *boundaryRebaseContext, lane numBoundLane, direction numbound.Direction) (numBoundLane, bool) {
	if lane.lane.Bottom() {
		return lane, true
	}
	values := make(map[keyspace.Key]int64, len(lane.lane.Values()))
	for path, value := range lane.lane.Values() {
		next, ok := boundaryRebasePaths(ctx, path)
		if !ok {
			return numBoundLane{}, false
		}
		for _, target := range next {
			candidate := value
			if existing, exists := values[target]; exists {
				if direction == numbound.Lower {
					candidate = min(existing, candidate)
				} else {
					candidate = max(existing, candidate)
				}
			}
			values[target] = candidate
		}
	}
	return numBoundLane{lane: lift.MustMapValues(values)}, true
}
func applyNumBoundBoundary(ctx *boundaryApplyContext, destination, fragment numBoundLane) (numBoundLane, bool) {
	if destination.lane.Bottom() || fragment.lane.Bottom() {
		return numBoundLane{lane: lift.MustMapBottom[keyspace.Key, int64]()}, true
	}
	values := applyFiniteMap(destination.lane.Values(), fragment.lane.Values(), func(path keyspace.Key, _ int64) bool { return ctx.closure.ContainsPath(path) })
	return numBoundLane{lane: lift.MustMapValues(values)}, true
}
func projectNumFloorsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.numFloors = projectNumBoundBoundary(ctx, source.numFloors)
	return true
}
func rebaseNumFloorsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseNumBoundBoundary(ctx, source.numFloors, numbound.Lower)
	out.numFloors = lane
	return ok
}
func applyNumFloorsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	lane, ok := applyNumBoundBoundary(ctx, destination.numFloors, fragment.numFloors)
	out.numFloors = lane
	return ok
}
func equalNumFloorsBoundary(_ *axis.Registry, a, b State) bool {
	return numBoundLaneDomain(numbound.Lower, nil).Equal(a.numFloors, b.numFloors)
}
func projectNumCeilsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.numCeils = projectNumBoundBoundary(ctx, source.numCeils)
	return true
}
func rebaseNumCeilsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseNumBoundBoundary(ctx, source.numCeils, numbound.Upper)
	out.numCeils = lane
	return ok
}
func applyNumCeilsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	lane, ok := applyNumBoundBoundary(ctx, destination.numCeils, fragment.numCeils)
	out.numCeils = lane
	return ok
}
func equalNumCeilsBoundary(_ *axis.Registry, a, b State) bool {
	return numBoundLaneDomain(numbound.Upper, nil).Equal(a.numCeils, b.numCeils)
}
