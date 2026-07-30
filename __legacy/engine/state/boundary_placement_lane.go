package state

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func projectPlacementBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.placement, _ = projectPlacementBoundaryFactor(ctx, source.placement)
	return true
}
func projectPlacementBoundaryFactor(ctx *boundaryProjectContext, source placementLane) (placementLane, bool) {
	if source.top {
		return source, true
	}
	values := projectFiniteMap(source.values, func(term identity.Term, _ placement.Value) bool { return ctx.closure.ContainsIdentityTerm(term) })
	return placementLane{mapLane[identity.Term, placement.Value]{values: values}}, true
}
func rebasePlacementBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.placement, ok = rebasePlacementBoundaryFactor(ctx, source.placement)
	return ok
}
func rebasePlacementBoundaryFactor(ctx *boundaryRebaseContext, source placementLane) (placementLane, bool) {
	if source.top {
		return source, true
	}
	values := make(map[identity.Term]placement.Value, len(source.values))
	for term, value := range source.values {
		image, ok := identityImage(ctx, term)
		if !ok {
			return placementLane{}, false
		}
		if image.IsBottom() {
			ctx.relationBottom = true
			return placementLane{}, true
		}
		if image.IsTop() {
			return placementLane{mapLane: mapLane[identity.Term, placement.Value]{top: true}}, true
		}
		next, exact := image.Term()
		if !exact {
			return placementLane{}, false
		}
		if existing, collision := values[next]; collision {
			value = placement.Lattice().Join(existing, value)
		}
		values[next] = value
	}
	return placementLane{mapLane[identity.Term, placement.Value]{values: values}}, true
}
func applyPlacementBoundaryLane(ctx *boundaryApplyContext, destination, fragment placementLane) (placementLane, bool) {
	if destination.top || fragment.top {
		return placementLane{mapLane: mapLane[identity.Term, placement.Value]{top: true}}, true
	}
	values := applyFiniteMap(destination.values, fragment.values, func(term identity.Term, _ placement.Value) bool { return ctx.closure.ContainsIdentityTerm(term) })
	return placementLane{mapLane[identity.Term, placement.Value]{values: values}}, true
}
func equalPlacementBoundary(_ *axis.Registry, a, b State) bool {
	return placementMapDomain().Equal(a.placement.asMap(placementMapDomain()), b.placement.asMap(placementMapDomain()))
}
