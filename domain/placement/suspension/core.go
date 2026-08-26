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
// needs no escape beyond its Stack baseline. A subject used after a
// suspension must survive the current activation, but no sharing/export proof
// is present, so OwnedHeap is the greatest precise demand available here.
// Unknown liveness still requires survival, but suspension alone can never
// imply sharing or export. Its concrete alternatives are DiesBefore (Stack)
// and Live (OwnedHeap), whose least Placement demand is OwnedHeap. The
// independent suspension-evidence factor retains the unknown proof polarity;
// writing Placement Top here would cross axes and poison a later authenticated
// SharedHeap publication.
func PlacementForState(state lifecycle.SubjectLivenessState) (placementdomain.Placement, bool) {
	switch state {
	case lifecycle.SubjectLivenessDiesBefore:
		return placementdomain.Stack, true
	case lifecycle.SubjectLivenessLive:
		return placementdomain.OwnedHeap, true
	case lifecycle.SubjectLivenessUnknown:
		return placementdomain.OwnedHeap, true
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
	index, indexOK := heap.AllocationKeyIndex(candidate.key)
	canonical, canonicalOK := schema.KeyAt(index)
	keyID, idOK := candidate.key.ContentID()
	_, _, allocationID, kind, form, originOK := heap.AllocationOriginForKey(candidate.key)
	if !indexOK || index < 0 || !canonicalOK || canonical != candidate.key || !idOK || !keyID.Available() ||
		originOK == false || !allocationID.Available() || !kind.Valid() || !form.Valid() {
		return operand{}, false
	}
	want, wantOK := PlacementForState(candidate.state)
	if !wantOK || !validPlacement(want) {
		return operand{}, false
	}
	return candidate, true
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

	// A widened plan is a view over the owner's immutable dense Heap
	// coordinates. It deliberately does not retain a copied allocation-root
	// catalogue. The schema is the sole coordinate authority and is safe to
	// carry by value across concurrent rule invocations.
	allRoot          bool
	allRootPrefix    bool
	allRootSchema    placementdomain.Schema
	allRootDenseSize int
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

func routeAtTag(plan routePlan, tag routeTag) (route, bool) {
	if plan.allRoot {
		return plan.allRootAtTag(tag)
	}
	low, high := 0, plan.count()
	for low < high {
		middle := low + (high-low)/2
		candidate, ok := plan.at(middle)
		if !ok {
			return route{}, false
		}
		if candidate.tag < tag {
			low = middle + 1
			continue
		}
		high = middle
	}
	if low < plan.count() {
		candidate, ok := plan.at(low)
		if ok && candidate.tag == tag {
			return candidate, true
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
	if plan.allRoot {
		return plan.allRootAt(index)
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
	if plan == nil || plan.allRoot || plan.size < 0 || candidate.tag == 0 {
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

// allAllocationPlan is the plan-owned widening form. It counts and
// authenticates the owner's allocation coordinates once, but leaves route
// materialization lazy: at/routeAtTag derive each route directly from that
// same immutable schema. Only exact routes beyond the inline prefix allocate
// an overflow suffix.
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
		if key.Kind() != heapdomain.RootAllocation {
			plan.allRootPrefix = false
			continue
		}
		plan.size++
	}
	return plan, true
}

func (plan routePlan) allRootAt(index int) (route, bool) {
	if !plan.allRoot || index < 0 || index >= plan.size || !plan.allRootSchema.Valid() {
		return route{}, false
	}
	if plan.allRootPrefix {
		key, keyOK := plan.allRootSchema.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
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
		if key.Kind() != heapdomain.RootAllocation {
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
	if !plan.allRoot || tag == 0 || !plan.allRootSchema.Valid() {
		return route{}, false
	}
	dense := uint64(tag - 1)
	if dense >= uint64(plan.allRootDenseSize) {
		return route{}, false
	}
	key, keyOK := plan.allRootSchema.KeyAt(int(dense))
	if !keyOK || key.Kind() != heapdomain.RootAllocation {
		return route{}, false
	}
	return route{key: key, tag: tag}, true
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
		if !item.present && !values.Equal(item.fact, values.Bottom()) {
			// Sparse absence is admissible only when the selected Value owner
			// supplied its exact lattice Bottom. A zero, foreign, or otherwise
			// malformed absent cell remains unavailable evidence.
			return routePlan{}, false
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
	if plan == nil {
		return false, false, false
	}
	projection, projectionOK := placementdomain.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return false, false, false
	}
	if projection.IsBottom() {
		return false, true, true
	}
	if projection.IsTop() {
		return true, true, true
	}
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK {
			return false, false, false
		}
		dense, denseOK := schema.Heap().AllocationKeyIndex(key)
		if !denseOK || !plan.add(route{key: key, tag: routeTag(uint64(dense) + 1)}) {
			return false, false, false
		}
	}
	return projection.HasOpaque(), true, true
}
