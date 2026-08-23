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

// boundaryValueOrdinal recovers the authored member ordinal from the tag
// carried by a Selection. SelectionAt is ordered by resolved engine Unit, not
// by this logical member ordinal, so callers must validate the tag instead of
// treating the physical selection index as the member index.
func boundaryValueOrdinal(tag valueTag, count int) (int, bool) {
	if tag == 0 || count < 0 {
		return 0, false
	}
	index := uint64(tag - 1)
	if index >= uint64(count) || index > uint64(int(^uint(0)>>1)) {
		return 0, false
	}
	return int(index), true
}

func boundaryCoordinateForTag(boundary valuedomain.ReturnBoundary, tag valueTag) (valuedomain.Coordinate, bool) {
	if boundary.MemberCount() < 0 {
		return valuedomain.Coordinate{}, false
	}
	index, indexOK := boundaryValueOrdinal(tag, boundary.MemberCount())
	if !indexOK {
		return valuedomain.Coordinate{}, false
	}
	return boundary.MemberAt(index)
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

// newReturnFacts reserves an indexed fact plane. The plane is deliberately
// separate from append: Selection order is physical Unit order, while this
// buffer is authored member order recovered from each tag.
func newReturnFacts(count int) (returnFacts, bool) {
	if count < 0 {
		return returnFacts{}, false
	}
	facts := returnFacts{count: count}
	if count > len(facts.inline) {
		facts.spill = make([]returnFact, count-len(facts.inline))
	}
	return facts, true
}

func (facts *returnFacts) set(index int, item returnFact) bool {
	if facts == nil || !item.available || index < 0 || index >= facts.count {
		return false
	}
	if prior, priorOK := facts.at(index); priorOK && prior.available {
		return false
	}
	if index < len(facts.inline) {
		facts.inline[index] = item
		return true
	}
	spillIndex := index - len(facts.inline)
	if spillIndex < 0 || spillIndex >= len(facts.spill) {
		return false
	}
	facts.spill[spillIndex] = item
	return true
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

	// Widened plans are a view over the owner's immutable dense coordinates.
	// They deliberately do not retain a copied allocation-root catalogue.
	allRoot          bool
	allRootPrefix    bool
	allRootSchema    placementdomain.Schema
	allRootDenseSize int
}

func (plan routePlan) widened() bool { return plan.class == routeWidened }

func (plan routePlan) routeCount() int {
	if plan.count < 0 {
		return 0
	}
	return plan.count
}

func (plan routePlan) routeAt(index int) (route, bool) {
	if index < 0 || index >= plan.count {
		return route{}, false
	}
	if plan.allRoot {
		return plan.allRootAt(index)
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	spillIndex := index - len(plan.inline)
	if spillIndex < 0 || spillIndex >= len(plan.spill) {
		return route{}, false
	}
	return plan.spill[spillIndex], true
}

func (plan routePlan) allRootAt(index int) (route, bool) {
	if !plan.allRoot || index < 0 || index >= plan.count || plan.allRootDenseSize < 0 || !plan.allRootSchema.Valid() {
		return route{}, false
	}
	if plan.allRootPrefix {
		key, keyOK := plan.allRootSchema.KeyAt(index)
		if !keyOK || key.Kind() != heap.RootAllocation {
			return route{}, false
		}
		return route{key: key, tag: routeTag(uint64(index) + 1)}, true
	}
	ordinal := 0
	for dense := 0; dense < plan.allRootDenseSize; dense++ {
		key, keyOK := plan.allRootSchema.KeyAt(dense)
		if !keyOK {
			return route{}, false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		if ordinal == index {
			return route{key: key, tag: routeTag(uint64(dense) + 1)}, true
		}
		ordinal++
	}
	return route{}, false
}

func (plan routePlan) allRootAtTag(tag routeTag) (route, bool) {
	if !plan.allRoot || tag == 0 || plan.allRootDenseSize < 0 || !plan.allRootSchema.Valid() {
		return route{}, false
	}
	dense := uint64(tag - 1)
	if dense >= uint64(plan.allRootDenseSize) {
		return route{}, false
	}
	key, keyOK := plan.allRootSchema.KeyAt(int(dense))
	if !keyOK || key.Kind() != heap.RootAllocation {
		return route{}, false
	}
	return route{key: key, tag: tag}, true
}

// addRoute inserts one exact route in canonical dense-tag order and
// deduplicates repeated allocation atoms. The first overflow route allocates
// only the suffix; the inline prefix remains part of the returned plan value.
func (plan *routePlan) addRoute(candidate route) bool {
	if plan == nil || plan.allRoot || plan.count < 0 || candidate.tag == 0 {
		return false
	}
	position := 0
	for position < plan.count {
		current, currentOK := plan.routeAt(position)
		if !currentOK {
			return false
		}
		if current.tag == candidate.tag {
			return current.key == candidate.key
		}
		if current.tag > candidate.tag {
			break
		}
		position++
	}

	if plan.count < len(plan.inline) {
		for index := plan.count; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.count++
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
		plan.spill = append(plan.spill, route{})
		copy(plan.spill[1:], plan.spill[:len(plan.spill)-1])
		plan.spill[0] = carried
	} else {
		spillIndex := position - len(plan.inline)
		if spillIndex < 0 || spillIndex > len(plan.spill) {
			return false
		}
		plan.spill = append(plan.spill, route{})
		copy(plan.spill[spillIndex+1:], plan.spill[spillIndex:len(plan.spill)-1])
		plan.spill[spillIndex] = candidate
	}
	plan.count++
	return true
}

// routePlanFor derives the only lawful return projection. Exact allocation
// references select their own Heap roots. Top and opaque alternatives widen to
// every Placement allocation root. Exact non-allocation roots (including Boot
// handles) and scalars produce no local route; Bottom remains distinct so the
// caller can reject the missing boundary value.
func routePlanFor(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (routePlan, bool) {
	projection, projectionOK := placementdomain.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return routePlan{}, false
	}
	if projection.IsBottom() {
		return routePlan{class: routeBottom}, true
	}
	if projection.IsTop() {
		routes, ok := allAllocationPlan(schema)
		if !ok {
			return routePlan{}, false
		}
		routes.class = routeWidened
		return routes, true
	}
	// Keep exact rooted alternatives in the bounded route representation itself.
	// Canonical insertion preserves dense-tag order and alias deduplication
	// without a temporary map, sort, or copied prefix.
	plan := routePlan{class: routeExact}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK {
			return routePlan{}, false
		}
		dense, denseOK := schema.Heap().AllocationKeyIndex(key)
		if !denseOK || !plan.addRoute(route{key: key, tag: routeTag(uint64(dense) + 1)}) {
			return routePlan{}, false
		}
	}
	if projection.HasOpaque() {
		allRoutes, ok := allAllocationPlan(schema)
		if !ok {
			return routePlan{}, false
		}
		allRoutes.class = routeWidened
		return allRoutes, true
	}
	if plan.routeCount() == 0 {
		return routePlan{class: routeScalar}, true
	}
	return plan, true
}

// routePlanForFacts joins the complete fixed ReturnBoundary Value selection
// into one Placement route plan. Exact allocation references are unioned
// across root and members; an opaque alternative, Top fact, or open tail
// widens to every allocation root. Missing selected cells and malformed Value
// facts are evidence failures, not semantic uncertainty: they refuse the
// route plan instead of fabricating a conservative all-roots result. A sparse
// cell is admissible only when Value supplied its exact owner-local Bottom;
// that is the Factor default and contributes no concrete return route. This is
// the one aggregate boundary where heterogeneous Value rows become a
// homogeneous Placement route set.
func routePlanForFacts(schema placementdomain.Schema, values *valuedomain.Schema, facts returnFacts, hasTail bool) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	plan := routePlan{class: routeExact}
	widen := false
	for index := 0; index < facts.len(); index++ {
		item, itemOK := facts.at(index)
		if !itemOK {
			return routePlan{}, false
		}
		if !authenticatedReturnFact(values, item.fact, item.present, item.available) {
			return routePlan{}, false
		}
		factWiden, factOK := collectValueRoutes(schema, values, item.fact, &plan)
		if !factOK {
			return routePlan{}, false
		}
		if factWiden {
			widen = true
		}
	}
	if hasTail || widen {
		routes, ok := allAllocationPlan(schema)
		if !ok {
			return routePlan{}, false
		}
		routes.class = routeWidened
		return routes, true
	}
	if plan.routeCount() == 0 {
		return routePlan{class: routeScalar}, true
	}
	return plan, true
}

// authenticatedReturnFact admits a present owner-issued Value or the exact
// sparse Bottom supplied by that same owner. It never constructs a Bottom from
// presence metadata, and therefore refuses unavailable, foreign, zero, or
// non-Bottom sparse cells.
func authenticatedReturnFact(values *valuedomain.Schema, fact valuedomain.Value, present, available bool) bool {
	if values == nil || !available || !values.Equal(fact, fact) {
		return false
	}
	return present || values.Equal(fact, values.Bottom())
}

// collectValueRoutes joins one exact Value fact directly into a caller-owned
// inline route plan. It preserves the Value non-smearing law and widens only
// when the fact contains an opaque or untracked reference alternative.
func collectValueRoutes(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value, plan *routePlan) (widen bool, ok bool) {
	if plan == nil {
		return false, false
	}
	projection, projectionOK := placementdomain.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return false, false
	}
	if projection.IsBottom() {
		return false, true
	}
	if projection.IsTop() {
		return true, true
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK {
			return false, false
		}
		dense, denseOK := schema.Heap().AllocationKeyIndex(key)
		if !denseOK || !plan.addRoute(route{key: key, tag: routeTag(uint64(dense) + 1)}) {
			return false, false
		}
	}
	return projection.HasOpaque(), true
}

// allAllocationPlan keeps widening lazy. It counts and authenticates the
// owner's allocation coordinates once, then routeAt/routeAtTag derive each
// selected route from that same immutable schema without copying a root
// catalogue.
func allAllocationPlan(schema placementdomain.Schema) (routePlan, bool) {
	if !schema.Valid() {
		return routePlan{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount < 0 {
		return routePlan{}, false
	}
	plan := routePlan{
		allRoot:          true,
		allRootPrefix:    true,
		allRootSchema:    schema,
		allRootDenseSize: denseCount,
	}
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return routePlan{}, false
		}
		if key.Kind() != heap.RootAllocation {
			plan.allRootPrefix = false
			continue
		}
		plan.count++
	}
	return plan, true
}

func routeAtTag(plan routePlan, tag routeTag) (route, bool) {
	if plan.allRoot {
		return plan.allRootAtTag(tag)
	}
	// Exact plans are emitted in Heap dense order, which is also the order of
	// their one-based route tags. Preserve the bounded route representation
	// while avoiding a linear scan for every staged row.
	low, high := 0, plan.routeCount()
	for low < high {
		middle := low + (high-low)/2
		candidate, candidateOK := plan.routeAt(middle)
		if !candidateOK {
			return route{}, false
		}
		if candidate.tag < tag {
			low = middle + 1
		} else {
			high = middle
		}
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

func returnValue(current placementdomain.Fact, present bool, plan routePlan) (placementdomain.Fact, bool) {
	current, currentOK := placementdomain.AuthenticateFactCell(current, present, true)
	if !currentOK {
		return placementdomain.BottomFact(), false
	}
	// Widening means that the Value identity names every possible allocation
	// root; it does not widen the known Return escape policy. Apply that policy
	// to each predecessor independently.
	return placementdomain.DisplaceFactChecked(current, placementdomain.Return)
}
