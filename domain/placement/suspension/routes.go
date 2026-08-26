package suspension

import (
	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one owner-issued Heap coordinate reached by a suspension subject.
// Key and destination are the same root for both class and evidence writers;
// the two projections remain separate declarations so the engine never
// reconstructs one from the other.
type Route struct {
	key heap.Key
	tag uint64
}

func (route Route) Coordinates() (heap.Key, heap.Key, bool) {
	return route.key, route.key, route.key.Valid() && route.key.Kind() == heap.RootAllocation && route.tag != 0
}

func (route Route) Predicate() (uint64, bool) {
	return route.tag, route.key.Valid() && route.key.Kind() == heap.RootAllocation && route.tag != 0
}

// RoutePlan is the relation-owned sealed route state. It wraps the existing
// canonical route algebra; it does not retain a catalog, inverse map, or
// second coordinate authority.
type RoutePlan struct{ plan routePlan }

// BoundaryCallKey is the exact Call coordinate a suspension subject is
// anchored at: the mounted call occurrence carried by the boundary at the
// span's lower endpoint. Program names the occurrence; Call owns the key. A
// boundary whose occurrence is not an admitted mounted call has no
// authenticated Call evidence and is refused rather than assumed to yield.
func BoundaryCallKey(calls *calldomain.Algebra, candidate lifecycle.MountedSubjectLiveness) (calldomain.Key, bool) {
	if calls == nil || !calls.Valid() || !candidate.Available() {
		return calldomain.Key{}, false
	}
	callID := candidate.BoundaryCallID()
	if !callID.Available() {
		return calldomain.Key{}, false
	}
	_, key, keyOK := calls.MountedCallKeyForOccurrence(candidate.MountID(), callID)
	if !keyOK {
		return calldomain.Key{}, false
	}
	return key, true
}

// DeriveSuspensionRoutes is the sole suspension route derivation, and the sole
// place the yield boundary is decided.
//
// The Call fact is an exact solve-time prerequisite. Call owns the dynamic
// target set and Target owns each operation's sealed suspension denominator,
// so MaySuspend is the whole gate: a call whose every target operation
// declares only a normal outcome cannot suspend, and its subject reaches no
// route at all. An opaque or non-operation alternative keeps the conservative
// answer, so opacity never elides a route.
//
// root is the subject's sealed allocation root when its owner issued one, and
// the zero key when the subject is planned from its Value source vector. That
// is the same projection the catalog sealed; this derivation authenticates the
// root against the schema but never re-opens the mounted Program to rebuild
// one.
//
// Missing or unauthenticated Call evidence refuses; an authenticated
// MaySuspend=false call produces a valid empty relation (NoSelection), never a
// fallback route.
func DeriveSuspensionRoutes(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate lifecycle.MountedSubjectLiveness,
	root heap.Key,
	callFact calldomain.Value,
	sources reduceroperand.SummaryVector[valuedomain.Value],
) (RoutePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) ||
		calls == nil || !calls.Valid() || !candidate.Available() || !sources.Valid() {
		return RoutePlan{}, false
	}
	callKey, callKeyOK := BoundaryCallKey(calls, candidate)
	if !callKeyOK {
		return RoutePlan{}, false
	}
	maySuspend, maySuspendOK := calls.MaySuspend(callKey, callFact)
	if !maySuspendOK {
		return RoutePlan{}, false
	}
	if !maySuspend {
		// Source cells cannot affect a call which Call has proved synchronous.
		// In particular, an unavailable source delivery must not turn the
		// canonical empty relation into a refusal.
		return RoutePlan{plan: routePlan{class: routeScalar}}, true
	}
	if root != (heap.Key{}) {
		plan, planOK := exactRootPlan(schema, root, sources)
		if !planOK {
			return RoutePlan{}, false
		}
		return RoutePlan{plan: plan}, true
	}
	plan, planOK := routePlanForSources(schema, values, sources)
	if !planOK {
		return RoutePlan{}, false
	}
	return RoutePlan{plan: plan}, true
}

// exactRootPlan is the single-route plan for a subject whose owner issued an
// allocation root directly. Such a subject publishes no Value source vector,
// so a delivered cell is malformed evidence rather than a wider route.
func exactRootPlan(schema placementdomain.Schema, root heap.Key, sources reduceroperand.SummaryVector[valuedomain.Value]) (routePlan, bool) {
	if !schema.Valid() || root.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(root) || sources.Count() != 0 {
		return routePlan{}, false
	}
	dense, denseOK := schema.Heap().AllocationKeyIndex(root)
	canonical, canonicalOK := schema.KeyAt(dense)
	if !denseOK || dense < 0 || !canonicalOK || canonical != root {
		return routePlan{}, false
	}
	var plan routePlan
	if !plan.add(route{key: root, tag: routeTag(uint64(dense) + 1)}) {
		return routePlan{}, false
	}
	plan.class = routeExact
	return plan, true
}

func SuspensionRouteCount(plan RoutePlan) int { return plan.plan.count() }

func SuspensionRouteAt(plan RoutePlan, index int) (Route, bool) {
	row, rowOK := plan.plan.at(index)
	if !rowOK {
		return Route{}, false
	}
	return Route{key: row.key, tag: uint64(row.tag)}, true
}
