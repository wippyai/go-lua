package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func membershipTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value KeyMembership) bool {
	return value.Key != "" && boundaryContainsStateKey(keys, closure, value.Key) ||
		value.Container.Kind != keyspace.KindInvalid && closure.ContainsPath(value.Container) ||
		boundaryContainsStateKey(keys, closure, value.Table)
}
func valueOriginTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexValueOrigin) bool {
	return boundaryContainsStateKey(keys, closure, value.Value) || closure.ContainsPath(value.Container)
}
func readOriginTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value DynamicIndexReadOrigin) bool {
	return boundaryContainsStateKey(keys, closure, value.Value) || closure.ContainsPath(value.Container) || boundaryContainsStateKey(keys, closure, value.Key)
}
func restoreTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value PendingDynamicAllValueRestore) bool {
	return closure.ContainsPath(value.Container) || boundaryContainsStateKey(keys, closure, value.Table) || boundaryContainsStateKey(keys, closure, value.Key)
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
func rebaseMembership(ctx *boundaryRebaseContext, value KeyMembership) ([]KeyMembership, bool) {
	keys, ok := optionalBoundaryStateKeys(ctx, value.Key)
	if !ok {
		return nil, false
	}
	containers, ok := optionalBoundaryPaths(ctx, value.Container)
	if !ok {
		return nil, false
	}
	tables, ok := rebaseBoundaryStateKeys(ctx, value.Table)
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
func rebaseReadOrigin(ctx *boundaryRebaseContext, value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
	values, ok := rebaseBoundaryStateKeys(ctx, value.Value)
	if !ok {
		return nil, false
	}
	containers, ok := boundaryRebasePaths(ctx, value.Container)
	if !ok {
		return nil, false
	}
	keys, ok := rebaseBoundaryStateKeys(ctx, value.Key)
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
func rebaseRestore(ctx *boundaryRebaseContext, value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
	containers, ok := boundaryRebasePaths(ctx, value.Container)
	if !ok {
		return nil, false
	}
	tables, ok := rebaseBoundaryStateKeys(ctx, value.Table)
	if !ok {
		return nil, false
	}
	keys, ok := rebaseBoundaryStateKeys(ctx, value.Key)
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
func projectKeyMembershipsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.keyMemberships.bottom {
		out.keyMemberships = source.keyMemberships
		return true
	}
	if source.keyMemberships.dynamicTop {
		out.keyMemberships = source.keyMemberships
		return true
	}
	out.keyMemberships = keyMembershipLane{
		path:            projectFiniteSet(source.keyMemberships.path, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamic:         projectFiniteSet(source.keyMemberships.dynamic, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamicAll:      projectFiniteSet(source.keyMemberships.dynamicAll, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		valueOrigins:    projectFiniteSet(source.keyMemberships.valueOrigins, func(value DynamicIndexValueOrigin) bool { return valueOriginTouches(ctx.keys, ctx.closure, value) }),
		readOrigins:     projectFiniteSet(source.keyMemberships.readOrigins, func(value DynamicIndexReadOrigin) bool { return readOriginTouches(ctx.keys, ctx.closure, value) }),
		pendingRestores: projectFiniteSet(source.keyMemberships.pendingRestores, func(value PendingDynamicAllValueRestore) bool { return restoreTouches(ctx.keys, ctx.closure, value) }),
	}
	return true
}
func rebaseKeyMembershipsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.keyMemberships.bottom {
		out.keyMemberships = source.keyMemberships
		return true
	}
	if source.keyMemberships.dynamicTop {
		out.keyMemberships = source.keyMemberships
		return true
	}
	next := keyMembershipLane{}
	var ok bool
	if next.path, ok = rekeySetMany(source.keyMemberships.path, func(value KeyMembership) ([]KeyMembership, bool) { return rebaseMembership(ctx, value) }); !ok {
		return false
	}
	if next.dynamic, ok = rekeySetMany(source.keyMemberships.dynamic, func(value KeyMembership) ([]KeyMembership, bool) { return rebaseMembership(ctx, value) }); !ok {
		return false
	}
	if next.dynamicAll, ok = rekeySetMany(source.keyMemberships.dynamicAll, func(value KeyMembership) ([]KeyMembership, bool) { return rebaseMembership(ctx, value) }); !ok {
		return false
	}
	if next.valueOrigins, ok = rekeySetMany(source.keyMemberships.valueOrigins, func(value DynamicIndexValueOrigin) ([]DynamicIndexValueOrigin, bool) {
		return rebaseValueOrigin(ctx, value)
	}); !ok {
		return false
	}
	if next.readOrigins, ok = rekeySetMany(source.keyMemberships.readOrigins, func(value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
		return rebaseReadOrigin(ctx, value)
	}); !ok {
		return false
	}
	if next.pendingRestores, ok = rekeySetMany(source.keyMemberships.pendingRestores, func(value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
		return rebaseRestore(ctx, value)
	}); !ok {
		return false
	}
	out.keyMemberships = next
	return true
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
func applyKeyMembershipsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.keyMemberships.bottom || fragment.keyMemberships.bottom {
		out.keyMemberships = keyMembershipLane{bottom: true}
		return true
	}
	if destination.keyMemberships.dynamicTop || fragment.keyMemberships.dynamicTop {
		out.keyMemberships = keyMembershipLane{dynamicTop: true}
		return true
	}
	out.keyMemberships = keyMembershipLane{
		path:            applyFiniteSet(destination.keyMemberships.path, fragment.keyMemberships.path, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamic:         applyFiniteSet(destination.keyMemberships.dynamic, fragment.keyMemberships.dynamic, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		dynamicAll:      applyFiniteSet(destination.keyMemberships.dynamicAll, fragment.keyMemberships.dynamicAll, func(value KeyMembership) bool { return membershipTouches(ctx.keys, ctx.closure, value) }),
		valueOrigins:    applyFiniteSet(destination.keyMemberships.valueOrigins, fragment.keyMemberships.valueOrigins, func(value DynamicIndexValueOrigin) bool { return valueOriginTouches(ctx.keys, ctx.closure, value) }),
		readOrigins:     applyFiniteSet(destination.keyMemberships.readOrigins, fragment.keyMemberships.readOrigins, func(value DynamicIndexReadOrigin) bool { return readOriginTouches(ctx.keys, ctx.closure, value) }),
		pendingRestores: applyFiniteSet(destination.keyMemberships.pendingRestores, fragment.keyMemberships.pendingRestores, func(value PendingDynamicAllValueRestore) bool { return restoreTouches(ctx.keys, ctx.closure, value) }),
	}
	return true
}
func equalKeyMembershipsBoundary(_ *axis.Registry, a, b State) bool {
	return keyMembershipLaneEqual(a.keyMemberships, b.keyMemberships)
}
