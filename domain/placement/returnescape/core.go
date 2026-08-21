package returnescape

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// routeTag is the stable semantic tag carried by one selected Placement
// route. It is a one-based Heap dense coordinate, not a second identity or
// coordinate space.
type routeTag uint64

// valueTag is the canonical heterogeneous ReturnBoundary member tag. Fixed
// members are carried in authored member order; the Values root is the exact
// anchor read and is deliberately not duplicated in this selected lane. It is
// transport evidence only and never a Value coordinate.
type valueTag uint64

type operand struct {
	boundary valuedomain.ReturnBoundary
	root     valuedomain.Coordinate
	id       identity.ContentID
}

// returnOperandForSchema resolves the exact Value-owned return boundary. The
// returned coordinate is the boundary's already-issued Value coordinate; this
// package never reconstructs a Program value row or occurrence projection.
func returnOperandForSchema(schema *valuedomain.Schema, module, occurrence identity.ContentID) (operand, bool) {
	if schema == nil || !schema.Valid() || !module.Available() || !occurrence.Available() {
		return operand{}, false
	}
	boundary, ok := schema.ReturnBoundary(module, occurrence)
	if !ok || !schema.OwnsReturnBoundary(boundary) {
		return operand{}, false
	}
	id, idOK := boundary.ID()
	root, rootOK := boundary.Root()
	if !idOK || !id.Available() || !rootOK {
		return operand{}, false
	}
	if _, indexOK := schema.CoordinateIndex(root); !indexOK {
		return operand{}, false
	}
	for index := 0; index < boundary.MemberCount(); index++ {
		member, memberOK := boundary.MemberAt(index)
		if !memberOK {
			return operand{}, false
		}
		if _, indexOK := schema.CoordinateIndex(member); !indexOK {
			return operand{}, false
		}
	}
	return operand{boundary: boundary, root: root, id: id}, true
}

func returnOperandContentForSchema(schema *valuedomain.Schema, candidate operand) (operand, [32]byte, bool) {
	if schema == nil || !schema.Valid() || !schema.OwnsReturnBoundary(candidate.boundary) || !candidate.id.Available() {
		return operand{}, [32]byte{}, false
	}
	id, idOK := candidate.boundary.ID()
	root, rootOK := candidate.boundary.Root()
	_, indexOK := schema.CoordinateIndex(root)
	if !idOK || !id.Available() || id != candidate.id || !rootOK || root != candidate.root || !indexOK {
		return operand{}, [32]byte{}, false
	}
	for index := 0; index < candidate.boundary.MemberCount(); index++ {
		member, memberOK := candidate.boundary.MemberAt(index)
		if !memberOK {
			return operand{}, [32]byte{}, false
		}
		if _, memberIndexOK := schema.CoordinateIndex(member); !memberIndexOK {
			return operand{}, [32]byte{}, false
		}
	}
	return candidate, [32]byte(id), true
}

func returnRootCoordinateForSchema(schema *valuedomain.Schema, candidate operand) (uint64, bool) {
	canonical, _, ok := returnOperandContentForSchema(schema, candidate)
	if !ok {
		return 0, false
	}
	index, indexOK := schema.CoordinateIndex(canonical.root)
	return uint64(index), indexOK
}

func boundaryValueTag(index int) (valueTag, bool) {
	if index < 0 {
		return 0, false
	}
	return valueTag(uint64(index) + 1), true
}

func boundaryCoordinateForTag(boundary valuedomain.ReturnBoundary, tag valueTag) (valuedomain.Coordinate, bool) {
	if tag == 0 {
		return valuedomain.Coordinate{}, false
	}
	index := uint64(tag - 1)
	if index > uint64(int(^uint(0)>>1)) {
		return valuedomain.Coordinate{}, false
	}
	return boundary.MemberAt(int(index))
}

type returnFact struct {
	fact      valuedomain.Value
	present   bool
	available bool
}

// returnFacts keeps the common fixed-return arity on the caller's stack. A
// Values row normally has only a handful of fixed members; larger rows spill
// to a slice and retain the same indexed view for the planner.
const returnFactsInlineCapacity = 4

type returnFacts struct {
	inline [returnFactsInlineCapacity]returnFact
	spill  []returnFact
	count  int
}

func (facts *returnFacts) append(item returnFact) {
	if facts.count < len(facts.inline) {
		facts.inline[facts.count] = item
	} else {
		facts.spill = append(facts.spill, item)
	}
	facts.count++
}

func (facts returnFacts) len() int { return facts.count }

func (facts returnFacts) at(index int) (returnFact, bool) {
	if index < 0 || index >= facts.count {
		return returnFact{}, false
	}
	if index < len(facts.inline) {
		return facts.inline[index], true
	}
	spillIndex := index - len(facts.inline)
	if spillIndex >= len(facts.spill) {
		return returnFact{}, false
	}
	return facts.spill[spillIndex], true
}

type route struct {
	key heap.Key
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
	class  routeClass
	inline [4]route
	spill  []route
	count  int
}

func (plan routePlan) widened() bool { return plan.class == routeWidened }

func (plan routePlan) routeCount() int { return plan.count }

func (plan routePlan) routeAt(index int) (route, bool) {
	if index < 0 || index >= plan.count {
		return route{}, false
	}
	if plan.spill != nil {
		return plan.spill[index], true
	}
	return plan.inline[index], true
}

func (plan *routePlan) appendRoute(candidate route) {
	if plan.spill != nil {
		plan.spill = append(plan.spill, candidate)
		plan.count++
		return
	}
	if plan.count < len(plan.inline) {
		plan.inline[plan.count] = candidate
		plan.count++
		return
	}
	plan.spill = make([]route, len(plan.inline), len(plan.inline)+1)
	copy(plan.spill, plan.inline[:])
	plan.spill = append(plan.spill, candidate)
	plan.count++
}

func (plan *routePlan) sortUnique() {
	if plan == nil || plan.count < 2 {
		return
	}
	routes := plan.inline[:plan.count]
	if plan.spill != nil {
		routes = plan.spill
	}
	// Keep the tiny exact-route case entirely on the caller's stack. Passing
	// the slice through sort.Interface makes the compiler conservatively move
	// the inline route plan to the heap even when one or two routes are all that
	// can occur on this hot path. Exact Value alternatives are already bounded
	// in practice, so insertion sort is both allocation-free and cheaper for
	// the small route sets this planner receives.
	for index := 1; index < len(routes); index++ {
		candidate := routes[index]
		cursor := index
		for cursor > 0 && routes[cursor-1].tag > candidate.tag {
			routes[cursor] = routes[cursor-1]
			cursor--
		}
		routes[cursor] = candidate
	}
	unique := 0
	for _, candidate := range routes {
		if unique != 0 && routes[unique-1].tag == candidate.tag {
			continue
		}
		routes[unique] = candidate
		unique++
	}
	plan.count = unique
	if plan.spill != nil {
		plan.spill = plan.spill[:unique]
	}
}

// routePlanFor derives the only lawful return projection. Exact allocation
// references select their own Heap roots. Top and opaque alternatives widen to
// every Placement allocation root. Exact non-allocation roots (including Boot
// handles) and scalars produce no local route; Bottom remains distinct so the
// caller can reject the missing boundary value.
func routePlanFor(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	if fact.IsBottom() {
		return routePlan{class: routeBottom}, true
	}
	if fact.IsTop() {
		routes, ok := allAllocationRoutes(schema)
		if !ok {
			return routePlan{}, false
		}
		routes.class = routeWidened
		return routes, true
	}
	if !values.Equal(fact, fact) {
		return routePlan{}, false
	}
	// Keep exact rooted alternatives in the route representation itself. The
	// old Atoms-plus-index-map path paid for multiple temporary collections before
	// producing this same dense-tagged route set; sorting and compacting this
	// one buffer preserves alias deduplication without widening the owner
	// boundary.
	plan := routePlan{class: routeExact}
	widen := false
	heapSchema := schema.Heap()
	validAtoms := true
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			validAtoms = false
			break
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			validAtoms = false
			break
		}
		switch classification.Class {
		case placementdomain.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
				validAtoms = false
				break
			}
			index, indexOK := heapSchema.KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || index < 0 || !canonicalOK || canonical != key {
				validAtoms = false
				break
			}
			plan.appendRoute(route{key: key, tag: routeTag(uint64(index) + 1)})
		case placementdomain.AtomClassOpaque:
			widen = true
		}
	}
	if !validAtoms {
		return routePlan{}, false
	}
	if widen {
		allRoutes, ok := allAllocationRoutes(schema)
		if !ok {
			return routePlan{}, false
		}
		allRoutes.class = routeWidened
		return allRoutes, true
	}
	if plan.routeCount() == 0 {
		return routePlan{class: routeScalar}, true
	}
	plan.sortUnique()
	return plan, true
}

// routePlanForFacts joins the complete fixed ReturnBoundary Value selection
// into one Placement route plan. Exact allocation references are unioned
// across root and members; an opaque alternative, unavailable selected cell,
// Top fact, or open tail widens to every allocation root. This is the one
// aggregate boundary where heterogeneous Value rows become a homogeneous
// Placement route set.
func routePlanForFacts(schema placementdomain.Schema, values *valuedomain.Schema, facts returnFacts, hasTail bool) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	if hasTail {
		routes, ok := allAllocationRoutes(schema)
		if !ok {
			return routePlan{}, false
		}
		routes.class = routeWidened
		return routes, true
	}
	plan := routePlan{class: routeExact}
	for index := 0; index < facts.len(); index++ {
		item, itemOK := facts.at(index)
		if !itemOK {
			return routePlan{}, false
		}
		if !item.available {
			routes, ok := allAllocationRoutes(schema)
			if !ok {
				return routePlan{}, false
			}
			routes.class = routeWidened
			return routes, true
		}
		if !item.present {
			continue
		}
		widen, factOK := collectValueRoutes(schema, values, item.fact, &plan)
		if !factOK {
			routes, ok := allAllocationRoutes(schema)
			if !ok {
				return routePlan{}, false
			}
			routes.class = routeWidened
			return routes, true
		}
		if widen {
			routes, ok := allAllocationRoutes(schema)
			if !ok {
				return routePlan{}, false
			}
			routes.class = routeWidened
			return routes, true
		}
	}
	if plan.routeCount() == 0 {
		return routePlan{class: routeScalar}, true
	}
	plan.sortUnique()
	return plan, true
}

// collectValueRoutes joins one exact Value fact directly into a caller-owned
// inline route plan. It preserves the Value non-smearing law and widens only
// when the fact contains an opaque or untracked reference alternative.
func collectValueRoutes(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value, plan *routePlan) (widen bool, ok bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || plan == nil {
		return false, false
	}
	if fact.IsBottom() {
		return false, true
	}
	if fact.IsTop() {
		return true, true
	}
	if !values.Equal(fact, fact) {
		return false, false
	}
	heapSchema := schema.Heap()
	valid := true
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			valid = false
			break
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			valid = false
			break
		}
		switch classification.Class {
		case placementdomain.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
				valid = false
				break
			}
			index, indexOK := heapSchema.KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || index < 0 || !canonicalOK || canonical != key {
				valid = false
				break
			}
			plan.appendRoute(route{key: key, tag: routeTag(uint64(index) + 1)})
		case placementdomain.AtomClassOpaque:
			widen = true
		}
	}
	return widen, valid
}

func allAllocationRoutes(schema placementdomain.Schema) (routePlan, bool) {
	if !schema.Valid() {
		return routePlan{}, false
	}
	// AllocationAt is itself a dense Heap scan. Walking the allocation-only
	// projection by ordinal would therefore rescan the prefix for every root.
	// The widened path is allowed to enumerate cold roots, but it must do so in
	// one dense pass and retain Heap's canonical order.
	denseCount := schema.DenseKeyCount()
	plan := routePlan{class: routeWidened}
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return routePlan{}, false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		plan.appendRoute(route{key: key, tag: routeTag(uint64(dense) + 1)})
	}
	return plan, true
}

func routeAtTag(plan routePlan, tag routeTag) (route, bool) {
	// Exact and widened plans are emitted in Heap dense order, which is also
	// the order of their one-based route tags. Preserve the zero-allocation
	// route representation while avoiding an O(N) scan for every staged row.
	low, high := 0, plan.routeCount()
	for low < high {
		middle := low + (high-low)/2
		candidate, candidateOK := plan.routeAt(middle)
		if !candidateOK {
			return route{}, false
		}
		if candidate.tag < tag {
			low = middle + 1
			continue
		}
		high = middle
	}
	index := low
	if index < plan.routeCount() {
		candidate, candidateOK := plan.routeAt(index)
		if candidateOK && candidate.tag == tag {
			return candidate, true
		}
	}
	return route{}, false
}

func returnValue(current placementdomain.Placement, present bool, plan routePlan) (placementdomain.Placement, bool) {
	if plan.class == routeWidened {
		return placementdomain.Unknown, true
	}
	if !present {
		return placementdomain.Displace(placementdomain.Bottom, placementdomain.Return), true
	}
	return placementdomain.Displace(current, placementdomain.Return), true
}
