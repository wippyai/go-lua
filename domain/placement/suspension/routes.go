package suspension

import (
	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
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

// DeriveSuspensionRoutes is the sole suspension route derivation. The Call
// fact is an exact prerequisite supplied by the engine. Missing or
// unauthenticated Call evidence refuses; an authenticated MaySuspend=false
// call produces a valid empty relation (NoSelection), never a fallback route.
func DeriveSuspensionRoutes(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate lifecycle.MountedSubjectLiveness,
	callFact calldomain.Value,
	sources reduceroperand.SummaryVector[valuedomain.Value],
) (RoutePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) ||
		calls == nil || !calls.Valid() || !candidate.Available() || !sources.Valid() {
		return RoutePlan{}, false
	}
	callID := candidate.BoundaryCallID()
	if !callID.Available() {
		return RoutePlan{}, false
	}
	_, callKey, callOK := calls.MountedCallKeyForOccurrence(candidate.MountID(), callID)
	if !callOK {
		return RoutePlan{}, false
	}
	maySuspend, maySuspendOK := calls.MaySuspend(callKey, callFact)
	if !maySuspendOK {
		return RoutePlan{}, false
	}
	if !maySuspend {
		return RoutePlan{plan: routePlan{class: routeScalar}}, true
	}

	span := candidate.Span()
	if !span.Available() {
		return RoutePlan{}, false
	}
	if span.SubjectKind() == lifecycle.SubjectLivenessRoot || span.SubjectKind() == lifecycle.SubjectLivenessValue {
		key, keyOK := directRootKey(schema, candidate)
		if !keyOK {
			return RoutePlan{}, false
		}
		dense, denseOK := schema.Heap().AllocationKeyIndex(key)
		canonical, canonicalOK := schema.KeyAt(dense)
		if !denseOK || !canonicalOK || canonical != key {
			return RoutePlan{}, false
		}
		var plan routePlan
		if !plan.add(route{key: key, tag: routeTag(uint64(dense) + 1)}) {
			return RoutePlan{}, false
		}
		plan.class = routeExact
		return RoutePlan{plan: plan}, true
	}

	count := sources.Count()
	if count < 0 {
		return RoutePlan{}, false
	}
	var inline [sourceFactInlineWidth]sourceFact
	facts, factsOK := sourceFactBuffer(count, inline[:])
	if !factsOK {
		return RoutePlan{}, false
	}
	for index := 0; index < count; index++ {
		fact, present, available := sources.At(index)
		facts[index] = sourceFact{fact: fact, present: present, available: available}
	}
	plan, planOK := routePlanForFacts(schema, values, facts)
	if !planOK {
		return RoutePlan{}, false
	}
	return RoutePlan{plan: plan}, true
}

func directRootKey(schema placementdomain.Schema, candidate lifecycle.MountedSubjectLiveness) (heap.Key, bool) {
	if !schema.Valid() || !candidate.Available() {
		return heap.Key{}, false
	}
	span := candidate.Span()
	if !span.Available() {
		return heap.Key{}, false
	}
	allocationID := span.SubjectID()
	if span.SubjectKind() == lifecycle.SubjectLivenessRoot {
		view, viewOK := calltarget.NewView(candidate.State())
		if !viewOK {
			return heap.Key{}, false
		}
		target, targetOK := view.ForBody(allocationID)
		if !targetOK {
			return heap.Key{}, false
		}
		allocationID = target.AllocationID()
	}
	if !allocationID.Available() {
		return heap.Key{}, false
	}
	issuer, issuerOK := schema.Heap().OccurrenceMountForModule(candidate.MountID())
	if !issuerOK {
		return heap.Key{}, false
	}
	key, keyOK := issuer.AllocationRootForOccurrence(allocationID)
	if !keyOK || key.Kind() != heap.RootAllocation || !schema.Heap().OwnsKey(key) {
		return heap.Key{}, false
	}
	module, programID, canonicalAllocation, _, _, originOK := schema.Heap().AllocationOriginForKey(key)
	if !originOK || module != candidate.MountID() || !programID.Available() || canonicalAllocation != allocationID {
		return heap.Key{}, false
	}
	return key, true
}

func SuspensionRouteCount(plan RoutePlan) int { return plan.plan.count() }

func SuspensionRouteAt(plan RoutePlan, index int) (Route, bool) {
	row, rowOK := plan.plan.at(index)
	if !rowOK {
		return Route{}, false
	}
	return Route{key: row.key, tag: uint64(row.tag)}, true
}
