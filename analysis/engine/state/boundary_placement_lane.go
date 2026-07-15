package state

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func projectPlacementBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.placement.top {
		out.placement = source.placement
		return true
	}
	values := projectFiniteMap(source.placement.values, func(id identity.ID, _ placement.Value) bool { return ctx.closure.ContainsIdentity(id) })
	out.placement = placementLane{mapLane[identity.ID, placement.Value]{values: values}}
	return true
}
func rebasePlacementBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.placement.top {
		out.placement = source.placement
		return true
	}
	values := make(map[identity.ID]placement.Value, len(source.placement.values))
	for id, value := range source.placement.values {
		next, ok := RebaseBoundaryIdentity(ctx.allocations, id)
		if !ok {
			return false
		}
		values[next] = value
	}
	out.placement = placementLane{mapLane[identity.ID, placement.Value]{values: values}}
	return true
}
func applyPlacementBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.placement.top || fragment.placement.top {
		out.placement = placementLane{mapLane: mapLane[identity.ID, placement.Value]{top: true}}
		return true
	}
	values := applyFiniteMap(destination.placement.values, fragment.placement.values, func(id identity.ID, _ placement.Value) bool { return ctx.closure.ContainsIdentity(id) })
	out.placement = placementLane{mapLane[identity.ID, placement.Value]{values: values}}
	return true
}
func equalPlacementBoundary(_ *axis.Registry, a, b State) bool {
	return placementMapDomain().Equal(a.placement.asMap(placementMapDomain()), b.placement.asMap(placementMapDomain()))
}
