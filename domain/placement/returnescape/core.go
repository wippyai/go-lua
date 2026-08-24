package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// routeTag is the stable semantic tag carried by one selected Placement
// route. It is a one-based Heap dense coordinate, not a second identity or
// coordinate space.
type routeTag uint64

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

// Route is the routed Placement coordinate the ReturnRoutes relation
// publishes. Key and Destination are the same allocation root: a return
// escape reads and writes one Placement Fact at the root the Value evidence
// names, so the relation declares that one authenticated pair rather than two
// coordinates.
type Route struct {
	Key heap.Key
	Tag uint64
}

// Coordinates is the ReturnRoutes relation's Key/Destination accessor. It is
// a direct field projection; the relation authenticates the row before this
// is ever called.
func (route Route) Coordinates() (key, destination heap.Key, ok bool) {
	return route.Key, route.Key, route.Key.Valid() && route.Key.Kind() == heap.RootAllocation && route.Tag != 0
}

// Predicate is the ReturnRoutes relation's declared selection tag: the route
// coordinate this member is published at, paired with the destination
// Coordinates answers. A zero tag is not a route row.
func (route Route) Predicate() (uint64, bool) {
	return route.Tag, route.Tag != 0
}

// RoutePlan is the ReturnRoutes relation's declared Derivation state. It is a
// thin exported view over the same route-planning algebra the generated
// family's zero-allocation worker calls directly; Count/At never re-derive a
// route from the underlying evidence.
type RoutePlan struct {
	plan routePlan
}

func (plan RoutePlan) RouteCount() int { return plan.plan.routeCount() }

func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	candidate, ok := plan.plan.routeAt(index)
	if !ok {
		return Route{}, false
	}
	return Route{Key: candidate.key, Tag: uint64(candidate.tag)}, true
}

// DeriveReturnRoutes is the ReturnRoutes relation's declared Build. root is
// the exact anchor Value fact: an authenticated candidate prerequisite this
// route algebra does not fold into its own value, exactly as Store's
// StorageFold declares an unused source input. members are already the
// owner-resolved per-member Value facts, exactly as Store's DeriveRoutes
// receives an already-authenticated source: a sparse member has already
// resolved to the owner's exact Bottom, so no separate presence flag is
// needed here. It folds the delivered vector through the same
// routePlanForFacts algebra the generated family uses and is not itself on
// that per-invocation hot path.
func DeriveReturnRoutes(schema placementdomain.Schema, values *valuedomain.Schema, boundary valuedomain.ReturnBoundary, root valuedomain.Value, members execution.SummaryVector[valuedomain.Value]) (RoutePlan, bool) {
	_ = root
	if values == nil || !values.Valid() || !values.OwnsReturnBoundary(boundary) || !members.Valid() {
		return RoutePlan{}, false
	}
	facts, factsOK := newReturnFacts(members.Count())
	if !factsOK {
		return RoutePlan{}, false
	}
	for index := 0; index < members.Count(); index++ {
		member, present, cell := members.At(index)
		// A member vector is a closed denominator: every declared ordinal is a
		// cell. An absence is admitted only as the owner's own exact Bottom,
		// which is the one value a coordinate the Factor never wrote holds.
		if !cell || !authenticatedReturnFact(values, member, present, true) {
			return RoutePlan{}, false
		}
		if !facts.set(index, returnFact{fact: member, present: present, available: true}) {
			return RoutePlan{}, false
		}
	}
	plan, planOK := routePlanForFacts(schema, values, facts, boundary.HasTail())
	if !planOK {
		return RoutePlan{}, false
	}
	return RoutePlan{plan: plan}, true
}

// ReturnRouteCount is the ReturnRoutes relation's declared Derivation Count.
func ReturnRouteCount(plan RoutePlan) int { return plan.RouteCount() }

// ReturnRouteAt is the ReturnRoutes relation's declared Derivation At.
func ReturnRouteAt(plan RoutePlan, index int) (Route, bool) { return plan.RouteAt(index) }

func returnValue(current placementdomain.Fact, present bool, plan routePlan) (placementdomain.Fact, bool) {
	_ = plan
	current, currentOK := placementdomain.AuthenticateFactCell(current, present, true)
	if !currentOK {
		return placementdomain.BottomFact(), false
	}
	result, outcome := ReturnEscapeFold(1, current)
	return result, outcome == structure.Concrete
}

// ReturnEscapeFold is the one authored return-escape judgment. Route
// materialization chooses the destination; this reducer only applies the
// canonical Return displacement to the authenticated predecessor. A zero
// route tag is not a route row and therefore refuses rather than fabricating
// an Unknown fact.
func ReturnEscapeFold(routeTag uint64, current placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	if routeTag == 0 {
		return placementdomain.BottomFact(), structure.Refuse
	}
	current, currentOK := placementdomain.AuthenticateFactCell(current, true, true)
	if !currentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := placementdomain.DisplaceFactChecked(current, placementdomain.Return)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
