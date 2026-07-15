package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
)

func projectLenFloorsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.lenFloors.lane.Bottom() {
		out.lenFloors = source.lenFloors
		return true
	}
	values := projectFiniteMap(source.lenFloors.lane.Values(), func(path keyspace.Key, _ lenbound.Floor) bool { return ctx.closure.ContainsPath(path) })
	out.lenFloors = lenFloorLane{lane: lift.MustMapValues(values)}
	return true
}
func rebaseLenFloorsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.lenFloors.lane.Bottom() {
		out.lenFloors = source.lenFloors
		return true
	}
	values := make(map[keyspace.Key]lenbound.Floor, len(source.lenFloors.lane.Values()))
	domain := lenbound.MapDomain()
	for path, value := range source.lenFloors.lane.Values() {
		next, ok := boundaryRebasePaths(ctx, path)
		if !ok {
			return false
		}
		for _, target := range next {
			candidate := value
			if existing, exists := values[target]; exists {
				joined := domain.Join(lift.MustMapValues(map[keyspace.Key]lenbound.Floor{target: existing}), lift.MustMapValues(map[keyspace.Key]lenbound.Floor{target: candidate}))
				candidate = joined.Values()[target]
			}
			values[target] = candidate
		}
	}
	out.lenFloors = lenFloorLane{lane: lift.MustMapValues(values)}
	return true
}
func applyLenFloorsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.lenFloors.lane.Bottom() || fragment.lenFloors.lane.Bottom() {
		out.lenFloors = lenFloorLane{lane: lift.MustMapBottom[keyspace.Key, lenbound.Floor]()}
		return true
	}
	values := applyFiniteMap(destination.lenFloors.lane.Values(), fragment.lenFloors.lane.Values(), func(path keyspace.Key, _ lenbound.Floor) bool { return ctx.closure.ContainsPath(path) })
	out.lenFloors = lenFloorLane{lane: lift.MustMapValues(values)}
	return true
}
func equalLenFloorsBoundary(_ *axis.Registry, a, b State) bool {
	return lenFloorMapDomain().Equal(a.lenFloors, b.lenFloors)
}
