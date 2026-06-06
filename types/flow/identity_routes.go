package flow

import "github.com/wippyai/go-lua/types/constraint"

// IdentityAliasRoutePolicy selects which identity carriers are followed by an
// alias route query. RequireRemainder keeps exact aliases out of callers that
// only want descendant replay.
type IdentityAliasRoutePolicy struct {
	PathAliases       bool
	AssignmentOrigins bool
	RequireRemainder  bool
}

var (
	// IdentityAliasReadPolicy follows every identity carrier. It is appropriate
	// for read queries, where an exact alias and a descendant alias are both
	// useful alternate keys.
	IdentityAliasReadPolicy = IdentityAliasRoutePolicy{
		PathAliases:       true,
		AssignmentOrigins: true,
	}

	// IdentityAliasDescendantOriginPolicy preserves legacy write-replay
	// semantics: only descendant writes through assignment-origin aliases are
	// replayed as writes to their source path.
	IdentityAliasDescendantOriginPolicy = IdentityAliasRoutePolicy{
		AssignmentOrigins: true,
		RequireRemainder:  true,
	}
)

// IdentityAliasSources returns source addresses that currently denote the same
// runtime identity as target through assignment aliases. It follows both
// PathAliases and ValueOriginAssignmentAlias facts and preserves consumed
// descendant suffixes structurally.
func IdentityAliasSources(state PointState, target StableAddress) []StableAddress {
	return IdentityAliasSourcesWithPolicy(state, target, IdentityAliasReadPolicy)
}

// IdentityAliasSourcesWithPolicy returns source addresses selected by policy.
func IdentityAliasSourcesWithPolicy(state PointState, target StableAddress, policy IdentityAliasRoutePolicy) []StableAddress {
	if target.Key() == "" {
		return nil
	}
	var out []StableAddress
	seen := map[constraint.PathKey]struct{}{}
	add := func(addr StableAddress) {
		key := addr.Key()
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}
	if policy.PathAliases {
		for _, use := range state.PathAliases.AliasesCoveringAddress(target) {
			if policy.RequireRemainder && len(use.Remainder) == 0 {
				continue
			}
			source, ok := StableAddressFromKey(use.Alias.Source)
			if !ok {
				continue
			}
			source, ok = source.Append(use.Remainder)
			if ok {
				add(source)
			}
		}
	}
	if policy.AssignmentOrigins {
		for _, use := range state.ValueOrigins.OriginsCoveringAddress(target) {
			if use.Origin.Kind != ValueOriginAssignmentAlias {
				continue
			}
			if policy.RequireRemainder && len(use.Remainder) == 0 {
				continue
			}
			source, ok := StableAddressFromKey(use.Origin.Source)
			if !ok {
				continue
			}
			source, ok = source.Append(use.Remainder)
			if ok {
				add(source)
			}
		}
	}
	return out
}

// IdentityAliasClosure returns root plus every assignment-alias source reachable
// from it. The result is deterministic in breadth-first order and contains each
// stable address at most once.
func IdentityAliasClosure(state PointState, root StableAddress) []StableAddress {
	if root.Key() == "" {
		return nil
	}
	var out []StableAddress
	seen := map[constraint.PathKey]struct{}{}
	queue := []StableAddress{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		key := cur.Key()
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cur)
		queue = append(queue, IdentityAliasSources(state, cur)...)
	}
	return out
}
