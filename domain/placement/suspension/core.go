// Package suspension consumes Program's neutral subject-liveness rows and
// projects the consequence of crossing a suspension boundary onto Placement.
//
// The package owns the Placement policy for those rows, but it does not
// inspect Flow terms or reconstruct a call graph.  Heap is the only source of
// allocation coordinates; Program is joined at the mounted artifact boundary
// while the Link catalog is sealed.
package suspension

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// PlacementForState is the sole Placement interpretation of a neutral
// subject-liveness answer. A subject proven dead before every normal re-entry
// needs no escape beyond its seeded Stack placement. A subject used after a
// suspension must survive the current activation, but no sharing/export proof
// is present, so OwnedHeap is the greatest precise demand available here.
// Unknown is the lattice top and is never treated as frame-local.
func PlacementForState(state lifecycle.SubjectLivenessState) (placementdomain.Placement, bool) {
	switch state {
	case lifecycle.SubjectLivenessDiesBefore:
		return placementdomain.Stack, true
	case lifecycle.SubjectLivenessLive:
		return placementdomain.OwnedHeap, true
	case lifecycle.SubjectLivenessUnknown:
		return placementdomain.Unknown, true
	default:
		// An invalid lifecycle ordinal is not an authenticated Unknown fact.
		// Refuse it at the catalog/rule boundary instead of manufacturing the
		// lattice top as an error sentinel.
		return placementdomain.Bottom, false
	}
}

// operand is one mounted subject-liveness row projected to an existing Heap
// allocation root. The row identity is Link-scoped by Catalog; key and state
// remain together so an admission cannot replace either side independently.
type operand struct {
	key     heapdomain.Key
	id      identity.ContentID
	state   lifecycle.SubjectLivenessState
	sources []source
}

func validPlacement(value placementdomain.Placement) bool {
	switch value {
	case placementdomain.Bottom, placementdomain.Stack, placementdomain.OwnedHeap,
		placementdomain.SharedHeap, placementdomain.Unknown:
		return true
	default:
		return false
	}
}

func operandForSchema(schema placementdomain.Schema, candidate operand) (operand, bool) {
	if !schema.Valid() || !candidate.id.Available() || !candidate.state.Valid() {
		return operand{}, false
	}
	heap := schema.Heap()
	if !heap.Valid() || !heap.OwnsKey(candidate.key) || candidate.key.Kind() != heapdomain.RootAllocation {
		return operand{}, false
	}
	index, indexOK := heap.KeyIndex(candidate.key)
	canonical, canonicalOK := schema.KeyAt(index)
	keyID, idOK := candidate.key.ContentID()
	_, _, allocationID, kind, form, originOK := heap.AllocationOriginForKey(candidate.key)
	if !indexOK || index < 0 || !canonicalOK || canonical != candidate.key || !idOK || !keyID.Available() ||
		originOK == false || !allocationID.Available() || !kind.Valid() || !form.Valid() {
		return operand{}, false
	}
	if !validPlacement(mustPlacement(candidate.state)) {
		return operand{}, false
	}
	return candidate, true
}

func mustPlacement(state lifecycle.SubjectLivenessState) placementdomain.Placement {
	value, ok := PlacementForState(state)
	if !ok {
		return placementdomain.Bottom
	}
	return value
}

func occurrenceID(module, rowID identity.ContentID) (identity.ContentID, bool) {
	if !module.Available() || !rowID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("program/placement-suspension-occurrence-v1"))
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(rowID[:])
	return identity.ContentID(hash.Sum(nil)), true
}

// routeTag is the transport tag carried by a selected Placement route.  It
// is deliberately a one-based dense Heap coordinate, not a second identity
// or coordinate authority.  Keeping the tag equal to the canonical dense
// coordinate also makes the checker authenticate route order without a
// retained inverse map.
type routeTag uint64

type route struct {
	key heapdomain.Key
	tag routeTag
}

type routeClass uint8

const (
	routeInvalid routeClass = iota
	routeBottom
	routeExact
	routeScalar
	routeWidened
)

type routePlan struct {
	// Ordinary suspension rows name only a handful of Heap roots. Keep those
	// routes inside the plan value itself so returning a plan can never retain
	// a slice pointing at a caller's stack scratch. Wider rows use only the
	// overflow suffix, which is invocation-local and explicit.
	inline [routeInlineWidth]route
	extra  []route
	size   int
	class  routeClass
}

// Selected source and route rows are ordinarily short. Source observations
// use caller-owned stack storage; route plans own their fixed prefix by value.
// Wider mounted rows use invocation-local overflow rather than mutable scratch
// retained on HotRule, which could race across engine sessions.
const (
	sourceFactInlineWidth = 8
	routeInlineWidth      = 8
)

func (plan routePlan) widened() bool { return plan.class == routeWidened }

func (plan routePlan) bottom() bool { return plan.class == routeBottom }

func routeAtTag(plan routePlan, tag routeTag) (route, bool) {
	for index := 0; index < plan.size; index++ {
		candidate, ok := plan.at(index)
		if !ok {
			return route{}, false
		}
		if candidate.tag == tag {
			return candidate, true
		}
		if candidate.tag > tag {
			return route{}, false
		}
	}
	return route{}, false
}

func (plan routePlan) count() int {
	if plan.size < 0 {
		return 0
	}
	return plan.size
}

func (plan routePlan) at(index int) (route, bool) {
	if index < 0 || index >= plan.size {
		return route{}, false
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	extra := index - len(plan.inline)
	if extra < 0 || extra >= len(plan.extra) {
		return route{}, false
	}
	return plan.extra[extra], true
}

// add inserts one route into canonical dense Heap order and deduplicates
// aliases. A same-tag foreign key is malformed and fails closed.
func (plan *routePlan) add(candidate route) bool {
	if plan == nil || plan.size < 0 {
		return false
	}
	position := 0
	for position < plan.size {
		current, ok := plan.at(position)
		if !ok {
			return false
		}
		switch {
		case current.tag == candidate.tag:
			return current.key == candidate.key
		case current.tag > candidate.tag:
			goto insert
		default:
			position++
		}
	}

insert:
	if plan.size < len(plan.inline) {
		for index := plan.size; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.size++
		return true
	}

	// Keep the inline prefix and append only the overflow suffix. Inserting
	// before the boundary carries the old inline tail into that suffix; no
	// second complete route slice is created.
	if position < len(plan.inline) {
		carried := plan.inline[len(plan.inline)-1]
		for index := len(plan.inline) - 1; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.extra = append(plan.extra, route{})
		copy(plan.extra[1:], plan.extra[:len(plan.extra)-1])
		plan.extra[0] = carried
	} else {
		extra := position - len(plan.inline)
		if extra < 0 || extra > len(plan.extra) {
			return false
		}
		plan.extra = append(plan.extra, route{})
		copy(plan.extra[extra+1:], plan.extra[extra:len(plan.extra)-1])
		plan.extra[extra] = candidate
	}
	plan.size++
	return true
}

// allAllocationPlan is the plan-owned widening form. It avoids materializing
// a slice merely to hand it back through routePlan; only roots beyond the
// inline prefix allocate an overflow suffix.
func allAllocationPlan(schema placementdomain.Schema) (routePlan, bool) {
	if !schema.Valid() {
		return routePlan{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount < 0 {
		return routePlan{}, false
	}
	var plan routePlan
	for index := 0; index < denseCount; index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			return routePlan{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if !plan.add(route{key: key, tag: routeTag(uint64(index) + 1)}) {
			return routePlan{}, false
		}
	}
	return plan, true
}

// sourceFactBuffer returns a bounded caller-owned view for one selected
// source row. A negative count is malformed; wider valid rows use invocation
// local storage and are never retained by the rule.
func sourceFactBuffer(count int, inline []sourceFact) ([]sourceFact, bool) {
	if count < 0 {
		return nil, false
	}
	if count <= cap(inline) {
		return inline[:count], true
	}
	return make([]sourceFact, count), true
}

// routePlanForValue projects one exact mounted Value fact onto the existing
// Heap allocation roots.  Exact rooted atoms remain exact routes.  A Top
// relation or an opaque/untracked reference widens to all allocation roots;
// scalar atoms contribute no placement route.  The function intentionally
// rejects foreign Value/Heap owners instead of treating them as Bottom.
func routePlanForValue(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	if fact.IsBottom() {
		if !values.Equal(fact, values.Bottom()) {
			return routePlan{}, false
		}
		return routePlan{class: routeBottom}, true
	}
	if fact.IsTop() {
		if !values.Equal(fact, values.Top()) {
			return routePlan{}, false
		}
		plan, ok := allAllocationPlan(schema)
		if !ok {
			return routePlan{}, false
		}
		plan.class = routeWidened
		return plan, true
	}

	var plan routePlan
	widen, validAtoms, visited := collectValueRoutes(schema, values, fact, &plan)
	if !validAtoms || !visited {
		return routePlan{}, false
	}
	if widen {
		widened, ok := allAllocationPlan(schema)
		if !ok {
			return routePlan{}, false
		}
		widened.class = routeWidened
		return widened, true
	}
	if plan.count() == 0 {
		plan.class = routeScalar
		return plan, true
	}
	plan.class = routeExact
	return plan, true
}

type sourceFact struct {
	fact      valuedomain.Value
	present   bool
	available bool
}

// routePlanForFacts is the selected-read planner used by both suspension
// consumers. It unions exact Value support directly into the plan's canonical
// inline/overflow route storage. Heap ownership/canonical-key checks remain in
// collectValueRoutes; only the temporary map and post-hoc integer sort are
// removed.
func routePlanForFacts(schema placementdomain.Schema, values *valuedomain.Schema, facts []sourceFact) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	var plan routePlan
	for _, item := range facts {
		if !item.available {
			// A revoked/unavailable Value cell is not an opaque Value fact.
			// Refuse the operation; widening it would fabricate authority for
			// every allocation root.
			return routePlan{}, false
		}
		if !item.present {
			// An absent selected cell is the Value lattice's Bottom
			// (there is no reachable alternative), not an open bridge. It
			// contributes no Heap route and must not erase exact roots from
			// another Values member.
			continue
		}
		widen, factOK, visited := collectValueRoutes(schema, values, item.fact, &plan)
		if !factOK || !visited {
			return routePlan{}, false
		}
		if widen {
			plan, allOK := allAllocationPlan(schema)
			if !allOK {
				return routePlan{}, false
			}
			plan.class = routeWidened
			return plan, true
		}
	}
	if plan.count() == 0 {
		plan.class = routeScalar
		return plan, true
	}
	plan.class = routeExact
	return plan, true
}

// collectValueRoutes authenticates one Value fact and appends every exact
// allocation route in canonical dense order. Exact non-allocation roots (in
// particular Boot handles) contribute no local route; only opaque reference
// alternatives widen the denominator.
func collectValueRoutes(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value, plan *routePlan) (widen bool, valid bool, visited bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || plan == nil {
		return false, false, false
	}
	if fact.IsBottom() {
		return false, values.Equal(fact, values.Bottom()), true
	}
	if fact.IsTop() {
		return true, values.Equal(fact, values.Top()), true
	}
	if !values.Equal(fact, fact) {
		return false, false, false
	}
	validAtoms := true
	visited = true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			validAtoms = false
			visited = false
			break
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			validAtoms = false
			visited = false
			break atoms
		}
		switch classification.Class {
		case placementdomain.AtomClassAllocation:
			key := classification.Key
			if !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
				validAtoms = false
				visited = false
				break atoms
			}
			index, indexOK := schema.Heap().KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || index < 0 || !canonicalOK || canonical != key {
				validAtoms = false
				visited = false
				break atoms
			}
			if !plan.add(route{key: key, tag: routeTag(uint64(index) + 1)}) {
				validAtoms = false
				visited = false
				break atoms
			}
		case placementdomain.AtomClassOpaque:
			widen = true
		}
	}
	return widen, validAtoms, visited
}
