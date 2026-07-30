package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func boundaryStateKeyRegistered(keys *keyspace.KeySpace, value pathaddr.StateKey) bool {
	if keys == nil || value == "" {
		return false
	}
	_, ok := keys.FromStateKey(value.PathKey())
	return ok
}

func boundaryPathRegistered(keys *keyspace.KeySpace, value keyspace.Key) bool {
	if keys == nil || value.Kind == keyspace.KindInvalid {
		return false
	}
	_, ok := keys.SegmentsView(value)
	return ok
}

func membershipBoundaryRegistered(keys *keyspace.KeySpace, value KeyMembership) bool {
	if !boundaryStateKeyRegistered(keys, value.Table) {
		return false
	}
	switch value.Kind {
	case KeyMembershipPath:
		return boundaryStateKeyRegistered(keys, value.Key)
	case KeyMembershipDynamicIndexValue, KeyMembershipDynamicIndexAllValues:
		return boundaryPathRegistered(keys, value.Container)
	default:
		return false
	}
}

func valueOriginBoundaryRegistered(keys *keyspace.KeySpace, value DynamicIndexValueOrigin) bool {
	return boundaryStateKeyRegistered(keys, value.Value) && boundaryPathRegistered(keys, value.Container)
}

func readOriginBoundaryRegistered(keys *keyspace.KeySpace, value DynamicIndexReadOrigin) bool {
	return boundaryStateKeyRegistered(keys, value.Value) && boundaryPathRegistered(keys, value.Container) && boundaryStateKeyRegistered(keys, value.Key)
}

func restoreBoundaryRegistered(keys *keyspace.KeySpace, value PendingDynamicAllValueRestore) bool {
	return boundaryPathRegistered(keys, value.Container) && boundaryStateKeyRegistered(keys, value.Table) && boundaryStateKeyRegistered(keys, value.Key)
}

func membershipTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value KeyMembership) bool {
	return membershipBoundaryRegistered(keys, value) && (value.Key != "" && boundaryContainsStateKey(keys, closure, value.Key) ||
		value.Container.Kind != keyspace.KindInvalid && closure.ContainsPath(value.Container) ||
		boundaryContainsStateKey(keys, closure, value.Table))
}
func valueOriginTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexValueOrigin) bool {
	return valueOriginBoundaryRegistered(keys, value) && (boundaryContainsStateKey(keys, closure, value.Value) || closure.ContainsPath(value.Container))
}
func readOriginTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexReadOrigin) bool {
	return readOriginBoundaryRegistered(keys, value) && (boundaryContainsStateKey(keys, closure, value.Value) || closure.ContainsPath(value.Container) || boundaryContainsStateKey(keys, closure, value.Key))
}
func restoreTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value PendingDynamicAllValueRestore) bool {
	return restoreBoundaryRegistered(keys, value) && (closure.ContainsPath(value.Container) || boundaryContainsStateKey(keys, closure, value.Table) || boundaryContainsStateKey(keys, closure, value.Key))
}

// Boundary projection must emit only relations for which the sealed closure
// owns every structural endpoint. Whole-State projection establishes this by
// fixed-point expansion. Factorwise projection receives an already sealed
// closure, so a relation that merely touches one selected endpoint is not a
// transportable fragment and must not be handed to the total rebaser.
func membershipWithinBoundary(keys *keyspace.KeySpace, closure BoundaryClosure, value KeyMembership) bool {
	if !membershipBoundaryRegistered(keys, value) || !boundaryContainsStateKey(keys, closure, value.Table) {
		return false
	}
	switch value.Kind {
	case KeyMembershipPath:
		return boundaryContainsStateKey(keys, closure, value.Key)
	case KeyMembershipDynamicIndexValue, KeyMembershipDynamicIndexAllValues:
		return closure.ContainsPath(value.Container)
	default:
		return false
	}
}

func valueOriginWithinBoundary(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexValueOrigin) bool {
	return valueOriginBoundaryRegistered(keys, value) && boundaryContainsStateKey(keys, closure, value.Value) && closure.ContainsPath(value.Container)
}

func readOriginWithinBoundary(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexReadOrigin) bool {
	return readOriginBoundaryRegistered(keys, value) && boundaryContainsStateKey(keys, closure, value.Value) && closure.ContainsPath(value.Container) && boundaryContainsStateKey(keys, closure, value.Key)
}

func restoreWithinBoundary(keys *keyspace.KeySpace, closure BoundaryClosure, value PendingDynamicAllValueRestore) bool {
	return restoreBoundaryRegistered(keys, value) && closure.ContainsPath(value.Container) && boundaryContainsStateKey(keys, closure, value.Table) && boundaryContainsStateKey(keys, closure, value.Key)
}
func optionalBoundaryStateKeys(ctx *boundaryRebaseContext, value pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	if value == "" {
		return []pathaddr.StateKey{""}, true
	}
	return rebaseBoundaryStateKeys(ctx, value)
}
func optionalBoundaryPaths(ctx *boundaryRebaseContext, value keyspace.Key) ([]keyspace.Key, bool) {
	if value.Kind == keyspace.KindInvalid {
		return []keyspace.Key{{}}, true
	}
	return boundaryRebasePaths(ctx, value)
}

type boundaryStateKeyRelation func(pathaddr.StateKey) ([]pathaddr.StateKey, bool)
type boundaryPathRelation func(keyspace.Key) ([]keyspace.Key, bool)

func mapMembership(value KeyMembership, optionalState boundaryStateKeyRelation, optionalPath boundaryPathRelation, stateKeys boundaryStateKeyRelation) ([]KeyMembership, bool) {
	keys, ok := optionalState(value.Key)
	if !ok {
		return nil, false
	}
	containers, ok := optionalPath(value.Container)
	if !ok {
		return nil, false
	}
	tables, ok := stateKeys(value.Table)
	if !ok {
		return nil, false
	}
	out := make([]KeyMembership, 0, len(keys)*len(containers)*len(tables))
	for _, key := range keys {
		for _, container := range containers {
			for _, table := range tables {
				next := value
				next.Key, next.Container, next.Table = key, container, table
				out = append(out, next)
			}
		}
	}
	return out, true
}

func rebaseMembership(ctx *boundaryRebaseContext, value KeyMembership) ([]KeyMembership, bool) {
	return mapMembership(value, func(key pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
		return optionalBoundaryStateKeys(ctx, key)
	}, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return optionalBoundaryPaths(ctx, path)
	}, func(key pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
		return rebaseBoundaryStateKeys(ctx, key)
	})
}

func rebaseDynamicAllMembership(ctx *boundaryRebaseContext, value KeyMembership) ([]KeyMembership, bool) {
	images, ok := rebaseMembership(ctx, value)
	if !ok {
		return nil, false
	}
	sourceTable, sourceTableOK := ctx.fromKeys.FromStateKey(value.Table.PathKey())
	out := images[:0]
	for _, image := range images {
		if !sameMembershipEndpointRepresentation(value.Container, image.Container) {
			continue
		}
		if sourceTableOK {
			targetTable, targetTableOK := ctx.toKeys.FromStateKey(image.Table.PathKey())
			if !targetTableOK || !sameMembershipEndpointRepresentation(sourceTable, targetTable) {
				continue
			}
		}
		out = append(out, image)
	}
	return out, true
}

func preimageMembership(ctx *boundaryRebaseContext, value KeyMembership) ([]KeyMembership, bool) {
	return mapMembership(value, ctx.quotient.optionalStateKeyPreimages, ctx.quotient.optionalPathPreimages, ctx.quotient.stateKeyPreimages)
}

// preimageDynamicAllMembership keeps the inverse fiber in the representation
// on which the all-values theorem is stated. A call frame may bind both a bare
// structural root and its certified resolver view to one destination root;
// those are two addresses for one object, not two independent theorem worlds.
// Filtering by endpoint kind avoids manufacturing versioned theorem premises
// while leaving ordinary path/SSA quotient behavior unchanged.
func preimageDynamicAllMembership(ctx *boundaryRebaseContext, value KeyMembership) ([]KeyMembership, bool) {
	preimages, ok := preimageMembership(ctx, value)
	if !ok {
		return nil, false
	}
	targetTable, targetTableOK := ctx.toKeys.FromStateKey(value.Table.PathKey())
	out := preimages[:0]
	for _, preimage := range preimages {
		if !sameMembershipEndpointRepresentation(value.Container, preimage.Container) {
			continue
		}
		if targetTableOK {
			sourceTable, sourceTableOK := ctx.fromKeys.FromStateKey(preimage.Table.PathKey())
			if !sourceTableOK || !sameMembershipEndpointRepresentation(targetTable, sourceTable) {
				continue
			}
		}
		out = append(out, preimage)
	}
	return out, len(out) != 0
}

func sameMembershipEndpointRepresentation(target, source keyspace.Key) bool {
	switch target.Kind {
	case keyspace.KindResolverSym, keyspace.KindUnversionedSym:
		return source.Kind == target.Kind
	default:
		return true
	}
}

func rebaseValueOrigin(ctx *boundaryRebaseContext, value DynamicIndexValueOrigin) ([]DynamicIndexValueOrigin, bool) {
	values, ok := rebaseBoundaryStateKeys(ctx, value.Value)
	if !ok {
		return nil, false
	}
	containers, ok := boundaryRebasePaths(ctx, value.Container)
	if !ok {
		return nil, false
	}
	out := make([]DynamicIndexValueOrigin, 0, len(values)*len(containers))
	for _, v := range values {
		for _, c := range containers {
			next := value
			next.Value, next.Container = v, c
			out = append(out, next)
		}
	}
	return out, true
}
func mapReadOrigin(value DynamicIndexReadOrigin, stateKeys boundaryStateKeyRelation, paths boundaryPathRelation) ([]DynamicIndexReadOrigin, bool) {
	values, ok := stateKeys(value.Value)
	if !ok {
		return nil, false
	}
	containers, ok := paths(value.Container)
	if !ok {
		return nil, false
	}
	keys, ok := stateKeys(value.Key)
	if !ok {
		return nil, false
	}
	out := make([]DynamicIndexReadOrigin, 0, len(values)*len(containers)*len(keys))
	for _, v := range values {
		for _, c := range containers {
			for _, k := range keys {
				next := value
				next.Value, next.Container, next.Key = v, c, k
				out = append(out, next)
			}
		}
	}
	return out, true
}
func rebaseReadOrigin(ctx *boundaryRebaseContext, value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
	return mapReadOrigin(value, func(key pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
		return rebaseBoundaryStateKeys(ctx, key)
	}, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	})
}
func preimageReadOrigin(ctx *boundaryRebaseContext, value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
	return mapReadOrigin(value, ctx.quotient.stateKeyPreimages, ctx.quotient.pathPreimages)
}

func mapRestore(value PendingDynamicAllValueRestore, stateKeys boundaryStateKeyRelation, paths boundaryPathRelation) ([]PendingDynamicAllValueRestore, bool) {
	containers, ok := paths(value.Container)
	if !ok {
		return nil, false
	}
	tables, ok := stateKeys(value.Table)
	if !ok {
		return nil, false
	}
	keys, ok := stateKeys(value.Key)
	if !ok {
		return nil, false
	}
	out := make([]PendingDynamicAllValueRestore, 0, len(containers)*len(tables)*len(keys))
	for _, c := range containers {
		for _, table := range tables {
			for _, k := range keys {
				next := value
				next.Container, next.Table, next.Key = c, table, k
				out = append(out, next)
			}
		}
	}
	return out, true
}
func rebaseRestore(ctx *boundaryRebaseContext, value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
	return mapRestore(value, func(key pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
		return rebaseBoundaryStateKeys(ctx, key)
	}, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	})
}
func preimageRestore(ctx *boundaryRebaseContext, value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
	return mapRestore(value, ctx.quotient.stateKeyPreimages, ctx.quotient.pathPreimages)
}
func projectKeyMembershipsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.keyMemberships, _ = projectKeyMembershipsBoundaryFactor(ctx, source.keyMemberships)
	return true
}
func projectKeyMembershipsBoundaryFactor(ctx *boundaryProjectContext, source keyMembershipLane) (keyMembershipLane, bool) {
	if source.bottom || source.dynamicTop {
		return source, true
	}
	return normalizeKeyMembershipLane(keyMembershipLane{
		path:       projectFiniteSet(source.path, func(value KeyMembership) bool { return membershipWithinBoundary(ctx.keys, ctx.closure, value) }),
		dynamic:    projectFiniteSet(source.dynamic, func(value KeyMembership) bool { return membershipWithinBoundary(ctx.keys, ctx.closure, value) }),
		dynamicAll: projectFiniteSet(source.dynamicAll, func(value KeyMembership) bool { return membershipWithinBoundary(ctx.keys, ctx.closure, value) }),
		valueOrigins: projectFiniteSet(source.valueOrigins, func(value DynamicIndexValueOrigin) bool {
			return valueOriginWithinBoundary(ctx.keys, ctx.closure, value)
		}),
		readOrigins: projectFiniteSet(source.readOrigins, func(value DynamicIndexReadOrigin) bool { return readOriginWithinBoundary(ctx.keys, ctx.closure, value) }),
		pendingRestores: projectFiniteSet(source.pendingRestores, func(value PendingDynamicAllValueRestore) bool {
			return restoreWithinBoundary(ctx.keys, ctx.closure, value)
		}),
	}), true
}
func rebaseKeyMembershipsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.keyMemberships, ok = rebaseKeyMembershipsBoundaryFactor(ctx, source.keyMemberships)
	return ok
}
func rebaseKeyMembershipsBoundaryFactor(ctx *boundaryRebaseContext, source keyMembershipLane) (keyMembershipLane, bool) {
	if source.bottom || source.dynamicTop {
		return source, true
	}
	next := keyMembershipLane{}
	var ok bool
	if next.path, ok = rebaseBoundaryMustSet(source.path, func(value KeyMembership) ([]KeyMembership, bool) {
		return rebaseMembership(ctx, value)
	}, func(value KeyMembership) KeyMembership { return value }, func(value KeyMembership) ([]KeyMembership, bool) {
		return preimageMembership(ctx, value)
	}); !ok {
		return keyMembershipLane{}, false
	}
	if next.dynamic, ok = rekeySetMany(source.dynamic, func(value KeyMembership) ([]KeyMembership, bool) { return rebaseMembership(ctx, value) }); !ok {
		return keyMembershipLane{}, false
	}
	if next.dynamicAll, ok = rebaseBoundaryMustSet(source.dynamicAll, func(value KeyMembership) ([]KeyMembership, bool) {
		return rebaseDynamicAllMembership(ctx, value)
	}, func(value KeyMembership) KeyMembership { return value }, func(value KeyMembership) ([]KeyMembership, bool) {
		return preimageDynamicAllMembership(ctx, value)
	}); !ok {
		return keyMembershipLane{}, false
	}
	if next.valueOrigins, ok = rekeySetMany(source.valueOrigins, func(value DynamicIndexValueOrigin) ([]DynamicIndexValueOrigin, bool) {
		return rebaseValueOrigin(ctx, value)
	}); !ok {
		return keyMembershipLane{}, false
	}
	if next.readOrigins, ok = rebaseBoundaryMustSet(source.readOrigins, func(value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
		return rebaseReadOrigin(ctx, value)
	}, func(value DynamicIndexReadOrigin) DynamicIndexReadOrigin { return value }, func(value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
		return preimageReadOrigin(ctx, value)
	}); !ok {
		return keyMembershipLane{}, false
	}
	if next.pendingRestores, ok = rebaseBoundaryMustSet(source.pendingRestores, func(value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
		return rebaseRestore(ctx, value)
	}, func(value PendingDynamicAllValueRestore) PendingDynamicAllValueRestore { return value }, func(value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
		return preimageRestore(ctx, value)
	}); !ok {
		return keyMembershipLane{}, false
	}
	return next, true
}

func rekeySetMany[T comparable](source map[T]struct{}, mapper func(T) ([]T, bool)) (map[T]struct{}, bool) {
	var out map[T]struct{}
	for value := range source {
		next, ok := mapper(value)
		if !ok {
			return nil, false
		}
		if out == nil && len(next) != 0 {
			out = make(map[T]struct{})
		}
		for _, item := range next {
			out[item] = struct{}{}
		}
	}
	return out, true
}
func applyKeyMembershipsBoundaryLane(ctx *boundaryApplyContext, destination, fragment keyMembershipLane) (keyMembershipLane, bool) {
	if destination.bottom || fragment.bottom {
		return keyMembershipLane{bottom: true}, true
	}
	if destination.dynamicTop || fragment.dynamicTop {
		return keyMembershipLane{dynamicTop: true}, true
	}
	return normalizeKeyMembershipLane(keyMembershipLane{
		path:            applyFiniteSet(destination.path, fragment.path, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamic:         applyFiniteSet(destination.dynamic, fragment.dynamic, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamicAll:      applyFiniteSet(destination.dynamicAll, fragment.dynamicAll, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		valueOrigins:    applyFiniteSet(destination.valueOrigins, fragment.valueOrigins, func(value DynamicIndexValueOrigin) bool { return valueOriginTouches(ctx.keys, ctx.closure, value) }),
		readOrigins:     applyFiniteSet(destination.readOrigins, fragment.readOrigins, func(value DynamicIndexReadOrigin) bool { return readOriginTouches(ctx.keys, ctx.closure, value) }),
		pendingRestores: applyFiniteSet(destination.pendingRestores, fragment.pendingRestores, func(value PendingDynamicAllValueRestore) bool { return restoreTouches(ctx.keys, ctx.closure, value) }),
	}), true
}
func equalKeyMembershipsBoundary(_ *axis.Registry, a, b State) bool {
	return keyMembershipLaneEqual(a.keyMemberships, b.keyMemberships)
}
