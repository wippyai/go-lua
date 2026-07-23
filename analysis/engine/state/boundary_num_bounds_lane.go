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
	values, ok := rebaseBoundaryMustMap(lane.lane.Values(), func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	}, func(value int64) (int64, bool) { return value, true }, func(path keyspace.Key) keyspace.Key { return path }, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return ctx.quotient.pathPreimages(path)
	}, func(a, b int64) int64 {
		if direction == numbound.Lower {
			return min(a, b)
		}
		return max(a, b)
	})
	if !ok {
		return numBoundLane{}, false
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
	setStateNumFloors(out, projectNumBoundBoundary(ctx, source.numFloors))
	return true
}
func rebaseNumFloorsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseNumBoundBoundary(ctx, source.numFloors, numbound.Lower)
	setStateNumFloors(out, lane)
	return ok
}
func equalNumFloorsBoundary(_ *axis.Registry, a, b State) bool {
	return numBoundLaneDomain(numbound.Lower, nil).Equal(a.numFloors, b.numFloors)
}
func projectNumCeilsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	setStateNumCeils(out, projectNumBoundBoundary(ctx, source.numCeils))
	return true
}
func rebaseNumCeilsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseNumBoundBoundary(ctx, source.numCeils, numbound.Upper)
	setStateNumCeils(out, lane)
	return ok
}
func equalNumCeilsBoundary(_ *axis.Registry, a, b State) bool {
	return numBoundLaneDomain(numbound.Upper, nil).Equal(a.numCeils, b.numCeils)
}
