package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func projectStoreRelationsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.storeRelations, _ = projectStoreRelationsBoundaryFactor(ctx, source.storeRelations)
	return true
}
func projectStoreRelationsBoundaryFactor(ctx *boundaryProjectContext, source storeRelationLane) (storeRelationLane, bool) {
	if source.bottom {
		return source, true
	}
	values := projectFiniteSet(source.values, func(value StoreRelation) bool {
		return boundaryContainsStateKey(ctx.keys, ctx.closure, value.Source) || boundaryContainsStateKey(ctx.keys, ctx.closure, value.Into)
	})
	return storeRelationLane{mustSetLane[StoreRelation]{values: values}}, true
}
func rebaseStoreRelationsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.storeRelations, ok = rebaseStoreRelationsBoundaryFactor(ctx, source.storeRelations)
	return ok
}
func rebaseStoreRelationsBoundaryFactor(ctx *boundaryRebaseContext, source storeRelationLane) (storeRelationLane, bool) {
	if source.bottom {
		return source, true
	}
	values, ok := rebaseBoundaryMustSet(source.values, func(value StoreRelation) ([]StoreRelation, bool) {
		sources, ok := rebaseBoundaryStateKeys(ctx, value.Source)
		if !ok {
			return nil, false
		}
		intos, ok := rebaseBoundaryStateKeys(ctx, value.Into)
		if !ok {
			return nil, false
		}
		out := make([]StoreRelation, 0, len(sources)*len(intos))
		for _, sourceKey := range sources {
			for _, intoKey := range intos {
				next := value
				next.Source, next.Into = sourceKey, intoKey
				out = append(out, next)
			}
		}
		return out, true
	}, func(value StoreRelation) boundaryPair[pathaddr.StateKey, pathaddr.StateKey] {
		return boundaryPair[pathaddr.StateKey, pathaddr.StateKey]{first: value.Source, second: value.Into}
	}, func(value StoreRelation) ([]boundaryPair[pathaddr.StateKey, pathaddr.StateKey], bool) {
		sources, valid := ctx.quotient.stateKeyPreimages(value.Source)
		if !valid {
			return nil, false
		}
		intos, valid := ctx.quotient.stateKeyPreimages(value.Into)
		if !valid {
			return nil, false
		}
		return boundaryPairs(sources, intos), true
	})
	if !ok {
		return storeRelationLane{}, false
	}
	return storeRelationLane{mustSetLane[StoreRelation]{values: values}}, true
}
func applyStoreRelationsBoundaryLane(ctx *boundaryApplyContext, destination, fragment storeRelationLane) (storeRelationLane, bool) {
	if destination.bottom || fragment.bottom {
		return storeRelationLane{mustSetLane: mustSetLane[StoreRelation]{bottom: true}}, true
	}
	values := applyFiniteSet(destination.values, fragment.values, func(value StoreRelation) bool {
		return boundaryContainsStateKey(ctx.keys, ctx.closure, value.Source) || boundaryContainsStateKey(ctx.keys, ctx.closure, value.Into)
	})
	return storeRelationLane{mustSetLane[StoreRelation]{values: values}}, true
}
func equalStoreRelationsBoundary(_ *axis.Registry, a, b State) bool {
	return storeRelationDomain().Equal(a.storeRelations, b.storeRelations)
}
