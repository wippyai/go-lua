package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// equalityQuotientPaths returns the exact finite congruence class observed by
// the path-evidence carrier. Unobserved paths are singleton classes: equality
// publication cannot manufacture a spelling that the carrier did not prove.
func equalityQuotientPaths(quotient pathevidence.EqualityQuotient, path keyspace.Key) ([]keyspace.Key, bool) {
	if !quotient.Valid() || path.Kind == keyspace.KindInvalid {
		return nil, false
	}
	class, present := quotient.Class(path)
	if !present {
		return []keyspace.Key{path}, true
	}
	out := make([]keyspace.Key, 0, 2)
	if !quotient.RangeClass(class, func(candidate keyspace.Key) { out = append(out, candidate) }) || len(out) == 0 {
		return nil, false
	}
	return out, true
}

func equalityQuotientStateKeys(ks *keyspace.KeySpace, quotient pathevidence.EqualityQuotient, value pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	if value == "" {
		return []pathaddr.StateKey{""}, true
	}
	path, ok := ks.FromStateKey(value.PathKey())
	if !ok {
		// Equality quotients range only over structurally interned paths. An
		// opaque/non-path StateKey is outside that carrier and therefore belongs
		// to its exact singleton class; rejecting it would make an unrelated
		// key-membership fact invalidate the whole quotient transaction.
		return []pathaddr.StateKey{value}, true
	}
	paths, ok := equalityQuotientPaths(quotient, path)
	if !ok {
		return nil, false
	}
	out := make([]pathaddr.StateKey, 0, len(paths))
	seen := make(map[pathaddr.StateKey]struct{}, len(paths))
	for _, candidate := range paths {
		formatted := ks.Format(candidate)
		stateKey, valid := pathaddr.StateKeyFromPathKey(formatted)
		if !valid {
			continue
		}
		if _, duplicate := seen[stateKey]; duplicate {
			continue
		}
		seen[stateKey] = struct{}{}
		out = append(out, stateKey)
	}
	return out, len(out) != 0
}

func applyPathEqualityDynamicIndex(
	lane dynamicIndexLane,
	reg *axis.Registry,
	_ *keyspace.KeySpace,
	quotient pathevidence.EqualityQuotient,
) (dynamicIndexLane, bool, bool) {
	if reg == nil || !quotient.Valid() {
		return lane, false, false
	}
	if lane.top || len(lane.values) == 0 {
		return lane, false, true
	}
	domain := dynamicindex.Domain(reg)
	values := make(map[dynamicindex.Key]dynamicindex.Fact, len(lane.values))
	for key, fact := range lane.values {
		paths, ok := equalityQuotientPaths(quotient, key.Table)
		if !ok {
			return lane, false, false
		}
		for _, path := range paths {
			nextKey := key
			nextKey.Table = path
			candidate := fact
			if prior, present := values[nextKey]; present {
				candidate = domain.Join(prior, candidate)
			}
			values[nextKey] = candidate
		}
	}
	next := dynamicIndexLaneFromMap(dynamicindex.MapDomain(reg), values)
	return next, !dynamicindex.MapDomain(reg).Equal(next.asMap(dynamicindex.MapDomain(reg)), lane.asMap(dynamicindex.MapDomain(reg))), true
}

func applyPathEqualityKeyMemberships(
	lane keyMembershipLane,
	_ *axis.Registry,
	ks *keyspace.KeySpace,
	quotient pathevidence.EqualityQuotient,
) (keyMembershipLane, bool, bool) {
	if ks == nil || !ks.Valid() || !quotient.Valid() {
		return lane, false, false
	}
	if lane.bottom || lane.dynamicTop {
		return lane, false, true
	}
	stateKeys := func(value pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
		return equalityQuotientStateKeys(ks, quotient, value)
	}
	paths := func(value keyspace.Key) ([]keyspace.Key, bool) {
		return equalityQuotientPaths(quotient, value)
	}
	optionalPaths := func(value keyspace.Key) ([]keyspace.Key, bool) {
		if value.Kind == keyspace.KindInvalid {
			return []keyspace.Key{value}, true
		}
		return equalityQuotientPaths(quotient, value)
	}
	next := lane
	var ok bool
	if next.path, ok = rekeySetMany(lane.path, func(value KeyMembership) ([]KeyMembership, bool) {
		return mapMembership(value, stateKeys, optionalPaths, stateKeys)
	}); !ok {
		return lane, false, false
	}
	if next.dynamic, ok = rekeySetMany(lane.dynamic, func(value KeyMembership) ([]KeyMembership, bool) {
		return mapMembership(value, stateKeys, optionalPaths, stateKeys)
	}); !ok {
		return lane, false, false
	}
	if next.dynamicAll, ok = rekeySetMany(lane.dynamicAll, func(value KeyMembership) ([]KeyMembership, bool) {
		return mapMembership(value, stateKeys, optionalPaths, stateKeys)
	}); !ok {
		return lane, false, false
	}
	if next.valueOrigins, ok = rekeySetMany(lane.valueOrigins, func(value DynamicIndexValueOrigin) ([]DynamicIndexValueOrigin, bool) {
		values, valid := stateKeys(value.Value)
		if !valid {
			return nil, false
		}
		containers, valid := paths(value.Container)
		if !valid {
			return nil, false
		}
		out := make([]DynamicIndexValueOrigin, 0, len(values)*len(containers))
		for _, valueKey := range values {
			for _, container := range containers {
				candidate := value
				candidate.Value, candidate.Container = valueKey, container
				out = append(out, candidate)
			}
		}
		return out, true
	}); !ok {
		return lane, false, false
	}
	if next.readOrigins, ok = rekeySetMany(lane.readOrigins, func(value DynamicIndexReadOrigin) ([]DynamicIndexReadOrigin, bool) {
		return mapReadOrigin(value, stateKeys, paths)
	}); !ok {
		return lane, false, false
	}
	if next.pendingRestores, ok = rekeySetMany(lane.pendingRestores, func(value PendingDynamicAllValueRestore) ([]PendingDynamicAllValueRestore, bool) {
		return mapRestore(value, stateKeys, paths)
	}); !ok {
		return lane, false, false
	}
	return next, !keyMembershipLaneEqual(next, lane), true
}
