package flow

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
	var out stableAddressList
	applyRoutes := func(routes []sourceRoute) {
		for _, route := range routes {
			if policy.RequireRemainder && len(route.remainder) == 0 {
				continue
			}
			source, ok := route.appendedSource()
			if ok {
				out.Add(source)
			}
		}
	}
	if policy.PathAliases {
		applyRoutes(state.PathAliases.sourceRoutesCoveringAddress(target))
	}
	if policy.AssignmentOrigins {
		applyRoutes(state.ValueOrigins.assignmentAliasSourceRoutesCoveringAddress(target))
	}
	return out.Values()
}

// IdentityAliasClosure returns root plus every assignment-alias source reachable
// from it. The result is deterministic in breadth-first order and contains each
// stable address at most once.
func IdentityAliasClosure(state PointState, root StableAddress) []StableAddress {
	if root.Key() == "" {
		return nil
	}
	var out stableAddressList
	queue := []StableAddress{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if !out.Add(cur) {
			continue
		}
		queue = append(queue, IdentityAliasSources(state, cur)...)
	}
	return out.Values()
}
