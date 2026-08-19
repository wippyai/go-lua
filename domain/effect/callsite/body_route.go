package callsite

import (
	"slices"

	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// bodyRoute is the sealed runtime selector witness for one body role.
type bodyRoute struct {
	tag  uint64
	root effectfactor.Root
}

// orderBodyRoutes fixes the sole role-to-root route table in the order a
// staged Selection is published in. The engine canonicalizes selected routes
// by exact Unit then numeric tag, and this rule tags every route with the
// same Effect root coordinate its exact Ref is issued from, so ascending tag
// is that canonical order. Call's target order and Effect's root order are
// separate authorities, so a route ordinal is taken from Effect's.
//
// It returns the ordered table and, for each input position, the 1-based slot
// its route took. Two routes on one tag address one Selection ordinal and are
// refused.
func orderBodyRoutes(routes []bodyRoute) ([]bodyRoute, []uint32, bool) {
	if len(routes) == 0 {
		return nil, nil, true
	}
	if uint64(len(routes)) >= uint64(^uint32(0)) {
		return nil, nil, false
	}
	order := make([]uint32, len(routes))
	for index := range order {
		order[index] = uint32(index)
	}
	slices.SortFunc(order, func(left, right uint32) int {
		switch {
		case routes[left].tag < routes[right].tag:
			return -1
		case routes[left].tag > routes[right].tag:
			return 1
		default:
			return 0
		}
	})
	ordered := make([]bodyRoute, len(routes))
	slots := make([]uint32, len(routes))
	for position, source := range order {
		if position > 0 && routes[source].tag == ordered[position-1].tag {
			return nil, nil, false
		}
		ordered[position] = routes[source]
		slots[source] = uint32(position + 1)
	}
	return ordered, slots, true
}
