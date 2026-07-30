package state

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
)

func projectLenFloorsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	lane, _ := projectLenFloorsBoundaryFactor(ctx, source.lenFloors)
	setStateLenFloors(out, lane)
	return true
}
func projectLenFloorsBoundaryFactor(ctx *boundaryProjectContext, source lenFloorLane) (lenFloorLane, bool) {
	if source.lane.Bottom() {
		return source, true
	}
	values := projectFiniteMap(source.lane.Values(), func(path keyspace.Key, _ lenbound.Floor) bool { return ctx.closure.ContainsPath(path) })
	return lenFloorLane{lane: lift.MustMapValues(values)}, true
}
func rebaseLenFloorsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseLenFloorsBoundaryFactor(ctx, source.lenFloors)
	if ok {
		setStateLenFloors(out, lane)
	}
	return ok
}
func rebaseLenFloorsBoundaryFactor(ctx *boundaryRebaseContext, source lenFloorLane) (lenFloorLane, bool) {
	if source.lane.Bottom() {
		return source, true
	}
	values, ok := rebaseBoundaryMustMap(source.lane.Values(), func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	}, func(value lenbound.Floor) (lenbound.Floor, bool) { return value, true }, func(path keyspace.Key) keyspace.Key { return path }, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return ctx.quotient.pathPreimages(path)
	}, func(a, b lenbound.Floor) lenbound.Floor {
		return lenbound.Floor{Lo: min(a.Lo, b.Lo)}
	})
	if !ok {
		return lenFloorLane{}, false
	}
	return lenFloorLane{lane: lift.MustMapValues(values)}, true
}
func applyLenFloorsBoundaryLane(ctx *boundaryApplyContext, destination, fragment lenFloorLane) (lenFloorLane, bool) {
	if destination.lane.Bottom() || fragment.lane.Bottom() {
		return lenFloorLane{lane: lift.MustMapBottom[keyspace.Key, lenbound.Floor]()}, true
	}
	values := applyFiniteMap(destination.lane.Values(), fragment.lane.Values(), func(path keyspace.Key, _ lenbound.Floor) bool { return ctx.closure.ContainsPath(path) })
	return lenFloorLane{lane: lift.MustMapValues(values)}, true
}
func equalLenFloorsBoundary(_ *axis.Registry, a, b State) bool {
	return lenFloorMapDomain().Equal(a.lenFloors, b.lenFloors)
}
