package state

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

func projectStoreRelationsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.storeRelations.bottom {
		out.storeRelations = source.storeRelations
		return true
	}
	values := projectFiniteSet(source.storeRelations.values, func(value StoreRelation) bool {
		return boundaryContainsStateKey(ctx.keys, ctx.closure, value.Source) || boundaryContainsStateKey(ctx.keys, ctx.closure, value.Into)
	})
	out.storeRelations = storeRelationLane{mustSetLane[StoreRelation]{values: values}}
	return true
}
func rebaseStoreRelationsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.storeRelations.bottom {
		out.storeRelations = source.storeRelations
		return true
	}
	values := make(map[StoreRelation]struct{}, len(source.storeRelations.values))
	for value := range source.storeRelations.values {
		sources, ok := rebaseBoundaryStateKeys(ctx, value.Source)
		if !ok {
			return false
		}
		intos, ok := rebaseBoundaryStateKeys(ctx, value.Into)
		if !ok {
			return false
		}
		for _, sourceKey := range sources {
			for _, intoKey := range intos {
				next := value
				next.Source, next.Into = sourceKey, intoKey
				values[next] = struct{}{}
			}
		}
	}
	out.storeRelations = storeRelationLane{mustSetLane[StoreRelation]{values: values}}
	return true
}
func applyStoreRelationsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.storeRelations.bottom || fragment.storeRelations.bottom {
		out.storeRelations = storeRelationLane{mustSetLane: mustSetLane[StoreRelation]{bottom: true}}
		return true
	}
	values := applyFiniteSet(destination.storeRelations.values, fragment.storeRelations.values, func(value StoreRelation) bool {
		return boundaryContainsStateKey(ctx.keys, ctx.closure, value.Source) || boundaryContainsStateKey(ctx.keys, ctx.closure, value.Into)
	})
	out.storeRelations = storeRelationLane{mustSetLane[StoreRelation]{values: values}}
	return true
}
func equalStoreRelationsBoundary(_ *axis.Registry, a, b State) bool {
	return storeRelationDomain().Equal(a.storeRelations, b.storeRelations)
}
