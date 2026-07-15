package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func projectFrozenBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.frozenTables.bottom {
		out.frozenTables = source.frozenTables
		return true
	}
	out.frozenTables = frozenTableLane{mustSetLane[identity.ID]{values: projectFiniteSet(source.frozenTables.values, ctx.closure.ContainsIdentity)}}
	return true
}
func rebaseFrozenBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.frozenTables.bottom {
		out.frozenTables = source.frozenTables
		return true
	}
	values := make(map[identity.ID]struct{}, len(source.frozenTables.values))
	for id := range source.frozenTables.values {
		next, ok := RebaseBoundaryIdentity(ctx.allocations, id)
		if !ok {
			return false
		}
		values[next] = struct{}{}
	}
	out.frozenTables = frozenTableLane{mustSetLane[identity.ID]{values: values}}
	return true
}
func applyFrozenBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.frozenTables.bottom || fragment.frozenTables.bottom {
		out.frozenTables = frozenTableLane{mustSetLane: mustSetLane[identity.ID]{bottom: true}}
		return true
	}
	values := applyFiniteSet(destination.frozenTables.values, fragment.frozenTables.values, ctx.closure.ContainsIdentity)
	out.frozenTables = frozenTableLane{mustSetLane[identity.ID]{values: values}}
	return true
}
func equalFrozenBoundary(_ *axis.Registry, a, b State) bool {
	return frozenTableDomain().Equal(a.frozenTables, b.frozenTables)
}
