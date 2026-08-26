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

	"github.com/wippyai/go-lua/analysis/engine"
	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	calldomain "github.com/wippyai/go-lua/domain/call"
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
	key heapdomain.Key
	id  identity.ContentID
	// liveness is the redeemed Program row this operand projects. The rule
	// reads the Call fact at the boundary it names, so the row travels with
	// the projection rather than being rebuilt from a mounted Program in a
	// selector or fold.
	liveness lifecycle.MountedSubjectLiveness
	state    lifecycle.SubjectLivenessState
	sources  []source
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
	if !schema.Valid() || !candidate.id.Available() || !candidate.state.Valid() || !candidate.liveness.Available() {
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
	sourceCellInlineWidth = 8
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

// sourceCellBuffer returns a bounded caller-owned view for one selected
// source row. A negative count is malformed; wider valid rows use invocation
// local storage and are never retained by the rule.
func sourceCellBuffer(count int, inline []reduceroperand.MemberCell[valuedomain.Value]) ([]reduceroperand.MemberCell[valuedomain.Value], bool) {
	if count < 0 {
		return nil, false
	}
	if count <= cap(inline) {
		return inline[:count], true
	}
	return make([]reduceroperand.MemberCell[valuedomain.Value], count), true
}

// routePlanForSources is the source-vector planner shared by the derivation
// and by both suspension consumers. It unions exact Value support directly
// into the plan's canonical inline/overflow route storage. Heap
// ownership/canonical-key checks remain in collectValueRoutes.
func routePlanForSources(schema placementdomain.Schema, values *valuedomain.Schema, sources reduceroperand.SummaryVector[valuedomain.Value]) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || !sources.Valid() {
		return routePlan{}, false
	}
	var plan routePlan
	for index := 0; index < sources.Count(); index++ {
		fact, present, available := sources.At(index)
		if !available {
			// A revoked/unavailable Value cell is not an opaque Value fact.
			// Refuse the operation; widening it would fabricate authority for
			// every allocation root.
			return routePlan{}, false
		}
		if !present && !values.Equal(fact, values.Bottom()) {
			// Sparse absence is admissible only when the selected Value owner
			// supplied its exact lattice Bottom. A zero, foreign, or otherwise
			// malformed absent cell remains unavailable evidence.
			return routePlan{}, false
		}
		widen, factOK, visited := collectValueRoutes(schema, values, fact, &plan)
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

// selectorSourceVector delivers one operand's authenticated Value source
// vector inside a selector. A subject whose owner issued an allocation root
// directly publishes no source vector, so the empty vector is its complete
// delivery rather than an unread one. The transport tag order is authenticated
// here, against the catalog-sealed source list, before any cell reaches the
// route derivation.
func selectorSourceVector(
	context engine.SelectorContext,
	read engine.Read[engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]]],
	canonical operand,
	inline []reduceroperand.MemberCell[valuedomain.Value],
) (reduceroperand.SummaryVector[valuedomain.Value], bool) {
	if canonical.key.Kind() == heapdomain.RootAllocation {
		return reduceroperand.NewMemberVector(inline[:0])
	}
	selection, selectionOK := engine.SelectorRead(context, read)
	if !selectionOK {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != len(canonical.sources) {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	cells, cellsOK := sourceCellBuffer(count, inline)
	if !cellsOK {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	for index := 0; index < count; index++ {
		tag, ordered, selectedOK := engine.SelectorSelectionAt(context, selection, index)
		if !selectedOK || tag != canonical.sources[index].tag || ordered.Count() != 1 {
			return reduceroperand.SummaryVector[valuedomain.Value]{}, false
		}
		cell, cellOK := sourceCell(ordered)
		if !cellOK {
			return reduceroperand.SummaryVector[valuedomain.Value]{}, false
		}
		cells[index] = cell
	}
	return reduceroperand.NewMemberVector(cells)
}

// frameSourceVector is the fold-side delivery of the same vector. It is
// generic over the written Factor because both suspension producers consume
// the identical Value source relation.
func frameSourceVector[V any](
	frame engine.Frame[V, operand],
	selection engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]],
	canonical operand,
	inline []reduceroperand.MemberCell[valuedomain.Value],
) (reduceroperand.SummaryVector[valuedomain.Value], bool) {
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	if canonical.key.Kind() == heapdomain.RootAllocation {
		if count != 0 {
			return reduceroperand.SummaryVector[valuedomain.Value]{}, false
		}
		return reduceroperand.NewMemberVector(inline[:0])
	}
	if count != len(canonical.sources) {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	cells, cellsOK := sourceCellBuffer(count, inline)
	if !cellsOK {
		return reduceroperand.SummaryVector[valuedomain.Value]{}, false
	}
	for index := 0; index < count; index++ {
		tag, ordered, selectedOK := engine.SelectionAt(frame, selection, index)
		if !selectedOK || tag != canonical.sources[index].tag || ordered.Count() != 1 {
			return reduceroperand.SummaryVector[valuedomain.Value]{}, false
		}
		cell, cellOK := sourceCell(ordered)
		if !cellOK {
			return reduceroperand.SummaryVector[valuedomain.Value]{}, false
		}
		cells[index] = cell
	}
	return reduceroperand.NewMemberVector(cells)
}

// sourceCell authenticates one delivered Value cell. Absence is preserved as
// the cell's own presence bit; an unavailable cell is not evidence at all and
// refuses here rather than reaching the route planner as a wider answer.
func sourceCell(ordered engine.OrderedCells[valuedomain.Value]) (reduceroperand.MemberCell[valuedomain.Value], bool) {
	fact, present, available := ordered.At(0)
	if !available {
		return reduceroperand.MemberCell[valuedomain.Value]{}, false
	}
	return reduceroperand.MemberCell[valuedomain.Value]{Value: fact, Present: present}, true
}

// boundaryCallCoordinate is the dense Call coordinate of the yield boundary
// one operand is anchored at. Program names the boundary occurrence and Call
// owns the directory it addresses; no local inverse is retained.
func boundaryCallCoordinate(calls *calldomain.Algebra, candidate operand) (uint64, bool) {
	key, keyOK := BoundaryCallKey(calls, candidate.liveness)
	if !keyOK {
		return 0, false
	}
	index, indexOK := calls.KeyIndex(key)
	return uint64(index), indexOK && index >= 0
}

// admitBoundaryCall authenticates the exact typed Call predecessor before the
// derivation treats its sparse bit as semantic state. Call's owner supplies
// its Factor Default in an absent observation; this rule accepts that sparse
// form only when the observed value is equal under the same Algebra to its
// exact Bottom. A missing or malformed row has no value to authenticate and
// refuses; this helper never manufactures Bottom from the read metadata.
func admitBoundaryCall(calls *calldomain.Algebra, canonical operand, cells engine.OrderedCells[calldomain.Value]) (calldomain.Value, bool) {
	if calls == nil || !calls.Valid() || cells.Count() != 1 {
		return calldomain.Value{}, false
	}
	key, keyOK := BoundaryCallKey(calls, canonical.liveness)
	if !keyOK {
		return calldomain.Value{}, false
	}
	value, present, available := cells.At(0)
	if !available || !calls.Admits(key, value) {
		return calldomain.Value{}, false
	}
	if !present && !calls.Equal(value, calls.Bottom()) {
		return calldomain.Value{}, false
	}
	return value, true
}
