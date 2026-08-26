package capture

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// routeTag is shared by the two selected reads.  Source tags are the
// one-based capture ordinal; Placement route tags are the one-based dense
// Heap coordinate.  They are opaque engine evidence, not a second identity
// or coordinate space.
type RouteTag uint64

type Source struct {
	module     identity.ContentID
	id         identity.ContentID
	coordinate valuedomain.Coordinate
	tag        RouteTag
}

// Coordinate is the Value coordinate this capture source is read at. It is
// the key projection of the capture-source relation.
func (candidate Source) Coordinate() valuedomain.Coordinate { return candidate.coordinate }

// Predicate is the tag the selection stamps this row with: the one-based
// capture ordinal, which is what correlates a returned cell with the source
// it was selected for.
func (candidate Source) Predicate() uint64 { return uint64(candidate.tag) }

// operand is one closure allocation root together with the canonical Value
// coordinates named by its Program capture rows. Every source coordinate is
// authenticated before the operand is admitted; there is no unresolved or
// compensating operand state.
type operand struct {
	key        heapdomain.Key
	id         identity.ContentID
	coordinate uint64
	sources    []Source
}

func rootOperandForSchema(schema placementdomain.Schema, key heapdomain.Key) (operand, bool) {
	if !schema.Valid() || !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return operand{}, false
	}
	index, indexOK := schema.Heap().AllocationKeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(index)
	id, idOK := key.ContentID()
	_, _, allocationID, kind, form, originOK := schema.Heap().AllocationOriginForKey(key)
	if !indexOK || index < 0 || !canonicalOK || canonical != key || !idOK || !id.Available() ||
		!originOK || !allocationID.Available() || kind != heapdomain.AllocationClosure || !form.Valid() {
		return operand{}, false
	}
	return operand{key: key, id: id, coordinate: uint64(index)}, true
}

func sourceCanonical(values *valuedomain.Schema, candidate Source) (Source, bool) {
	if values == nil || !values.Valid() || !candidate.module.Available() || !candidate.id.Available() || candidate.tag == 0 {
		return Source{}, false
	}
	coordinate, coordinateOK := values.CoordinateForMountedSemantic(candidate.module, candidate.id)
	if !coordinateOK || coordinate != candidate.coordinate {
		return Source{}, false
	}
	if _, indexOK := values.CoordinateIndex(candidate.coordinate); !indexOK {
		return Source{}, false
	}
	return candidate, true
}

// sourceOrdinal recovers the authored capture position from a selected route
// tag. The engine exposes Selection rows in physical Unit order, so the
// selection ordinal is not the capture ordinal. A source tag is one-based and
// must name exactly one declared source.
func sourceOrdinal(candidate operand, tag RouteTag) (int, bool) {
	if tag == 0 || len(candidate.sources) == 0 {
		return 0, false
	}
	index := uint64(tag - 1)
	if index >= uint64(len(candidate.sources)) || index > uint64(int(^uint(0)>>1)) {
		return 0, false
	}
	logical := int(index)
	return logical, candidate.sources[logical].tag == tag
}

func operandContentForSchema(schema placementdomain.Schema, values *valuedomain.Schema, candidate operand) (operand, [32]byte, bool) {
	canonical, ok := rootOperandForSchema(schema, candidate.key)
	if !ok || candidate.id != canonical.id || candidate.coordinate != canonical.coordinate || values == nil || !values.Valid() {
		return operand{}, [32]byte{}, false
	}
	var seenTags map[RouteTag]struct{}
	if len(candidate.sources) > 1 {
		seenTags = make(map[RouteTag]struct{}, len(candidate.sources))
	}
	var previousTag RouteTag
	for index, item := range candidate.sources {
		checked, sourceOK := sourceCanonical(values, item)
		if !sourceOK {
			return operand{}, [32]byte{}, false
		}
		duplicate := false
		if seenTags != nil {
			_, duplicate = seenTags[checked.tag]
		}
		if duplicate || index > 0 && checked.tag <= previousTag {
			return operand{}, [32]byte{}, false
		}
		if seenTags != nil {
			seenTags[checked.tag] = struct{}{}
		}
		previousTag = checked.tag
	}
	// sourceCanonical authenticates each existing field without rewriting it;
	// retain the sealed source plane instead of allocating a duplicate slice
	// for every operand admission.
	canonical.sources = candidate.sources
	return canonical, [32]byte(canonical.id), true
}

// captureOperandForSchema admits only a complete, owner-authenticated capture
// boundary. A capture-free boundary is excluded deliberately; every positive
// capture boundary must resolve every declared source coordinate. Missing,
// malformed, or foreign program evidence refuses the operand instead of
// manufacturing an uncertain operand that widens all roots.
func captureOperandForSchema(schema placementdomain.Schema, values *valuedomain.Schema, base operand) (candidate operand, include, ok bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return operand{}, false, false
	}
	canonical, baseOK := rootOperandForSchema(schema, base.key)
	if !baseOK || base.id != canonical.id || base.coordinate != canonical.coordinate || len(base.sources) != 0 {
		return operand{}, false, false
	}
	sources, include, sourcesOK := DeriveCaptureSources(schema, values, canonical.key)
	if !sourcesOK || !include {
		return operand{}, false, sourcesOK
	}
	candidate = canonical
	candidate.sources = sources
	return candidate, true, true
}

// DeriveCaptureSources is the operation that publishes the capture-source
// rows of one closure allocation root: one row per declared capture, in the
// authored capture order, each carrying the canonical Value coordinate of the
// cell it closes over and the one-based ordinal that tags it.
//
// It takes the sealed inputs alone - the Placement schema, the Value schema,
// and the allocation key - so the rows a reading rule joins are produced by a
// statement of this owner and not by a traversal the declaration never named.
// include distinguishes a capture-free boundary, which is an excluded
// candidate rather than a refusal, from a boundary this owner cannot
// authenticate.
func DeriveCaptureSources(schema placementdomain.Schema, values *valuedomain.Schema, key heapdomain.Key) (sources []Source, include, ok bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return nil, false, false
	}
	canonical, canonicalOK := rootOperandForSchema(schema, key)
	if !canonicalOK {
		return nil, false, false
	}
	module, programID, allocationID, kind, _, originOK := schema.Heap().AllocationOriginForKey(canonical.key)
	if !originOK || kind != heapdomain.AllocationClosure || !module.Available() || !programID.Available() || !allocationID.Available() {
		return nil, false, false
	}
	mount, mountOK := schema.Heap().MountedArtifactForModule(module)
	if !mountOK || mount.ProgramID != programID || mount.Snapshot == nil {
		return nil, false, false
	}
	program := mount.Snapshot.Program()
	if !program.Available() {
		return nil, false, false
	}
	state, stateOK := program.ColdState()
	targets, targetsOK := calltarget.NewView(state)
	allocations, allocationsOK := heapallocation.NewView(state)
	target, targetOK := targets.ForAllocation(allocationID)
	allocation, allocationOK := allocations.AllocationForID(allocationID)
	boundary, boundaryOK := program.FunctionBoundaryForBody(target.BodyID())
	if !stateOK || !targetsOK || !allocationsOK || !targetOK || !allocationOK || allocation.Role() != heapallocation.RoleClosure || !boundaryOK {
		return nil, false, false
	}
	count := boundary.CaptureCount()
	if count == 0 {
		return nil, false, true
	}
	offset, spanCount, spanOK := boundary.CaptureSpan()
	if !spanOK || int(spanCount) != count || uint64(offset)+uint64(spanCount) > uint64(^uint32(0)) {
		return nil, false, false
	}
	rows := make([]Source, 0, count)
	for index := 0; index < count; index++ {
		capture, captureOK := program.FunctionCaptureAt(int(offset) + index)
		position, positionOK := capture.Position()
		if !captureOK || !positionOK || position != index || capture.InnerBodyID() != boundary.BodyID() {
			return nil, false, false
		}
		storageID := capture.OuterStorageCellID()
		if !storageID.Available() {
			return nil, false, false
		}
		coordinate, coordinateOK := values.CoordinateForMountedSemantic(module, storageID)
		item := Source{module: module, id: storageID, coordinate: coordinate, tag: RouteTag(uint64(index) + 1)}
		if !coordinateOK || !coordinate.Valid() {
			return nil, false, false
		}
		rows = append(rows, item)
	}
	if len(rows) != count {
		return nil, false, false
	}
	return rows, true, true
}

type Route struct {
	key heapdomain.Key
	tag RouteTag
}

// Coordinates are the cell this route reads and the cell it publishes into.
// A capture route moves nothing: the closure's placement is joined into the
// captured allocation's own cell, so both endpoints are that one key.
func (item Route) Coordinates() (heapdomain.Key, heapdomain.Key) { return item.key, item.key }

// Predicate is the tag the selection stamps this row with: the one-based
// dense Heap coordinate of the allocation the route names.
func (item Route) Predicate() uint64 { return uint64(item.tag) }

type routeClass uint8

const (
	routeBottom routeClass = iota
	routeExact
	routeScalar
	routeWidened
)

type RoutePlan struct {
	class  routeClass
	inline [captureRouteInlineCapacity]Route
	spill  []Route
	count  int

	// Widened plans are a view over the owner's canonical dense coordinates.
	// They deliberately do not retain a copied allocation-root catalogue.
	allRoot          bool
	allRootPrefix    bool
	allRootSchema    placementdomain.Schema
	allRootDenseSize int
}

// Capture route sets are ordinarily small. Keep exact routes in the plan
// value itself so routePlanForFacts does not allocate a map, ordering slice,
// or result slice for common captures. A wider exact set spills only its
// suffix; widened plans retain only the immutable owner-schema view above.
const captureRouteInlineCapacity = 8

func (plan RoutePlan) RouteCount() int {
	if plan.count < 0 {
		return 0
	}
	return plan.count
}

func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	if index < 0 || index >= plan.RouteCount() {
		return Route{}, false
	}
	if plan.allRoot {
		return plan.allRootAt(index)
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	spillIndex := index - len(plan.inline)
	if spillIndex < 0 || spillIndex >= len(plan.spill) {
		return Route{}, false
	}
	return plan.spill[spillIndex], true
}

func (plan RoutePlan) allRootAt(index int) (Route, bool) {
	if !plan.allRoot || index < 0 || index >= plan.count || !plan.allRootSchema.Valid() {
		return Route{}, false
	}
	if plan.allRootPrefix {
		key, keyOK := plan.allRootSchema.KeyAt(index)
		return Route{key: key, tag: RouteTag(uint64(index) + 1)}, keyOK && key.Kind() == heapdomain.RootAllocation
	}
	ordinal := 0
	for dense := 0; dense < plan.allRootDenseSize; dense++ {
		key, keyOK := plan.allRootSchema.KeyAt(dense)
		if !keyOK {
			return Route{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if ordinal == index {
			return Route{key: key, tag: RouteTag(uint64(dense) + 1)}, true
		}
		ordinal++
	}
	return Route{}, false
}

func (plan RoutePlan) allRootAtTag(tag RouteTag) (Route, bool) {
	if !plan.allRoot || tag == 0 || !plan.allRootSchema.Valid() {
		return Route{}, false
	}
	dense := uint64(tag - 1)
	if dense >= uint64(plan.allRootDenseSize) {
		return Route{}, false
	}
	key, keyOK := plan.allRootSchema.KeyAt(int(dense))
	if !keyOK || key.Kind() != heapdomain.RootAllocation {
		return Route{}, false
	}
	return Route{key: key, tag: tag}, true
}

// addRoute inserts one exact route in canonical dense-tag order and
// deduplicates repeated allocation atoms. The first overflow route allocates
// only the suffix; the inline prefix remains part of the returned plan value.
func (plan *RoutePlan) addRoute(candidate Route) bool {
	if plan == nil || plan.allRoot || plan.count < 0 || candidate.tag == 0 {
		return false
	}
	position := 0
	for position < plan.count {
		current, currentOK := plan.RouteAt(position)
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
	if position < len(plan.inline) {
		carried := plan.inline[len(plan.inline)-1]
		for index := len(plan.inline) - 1; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.spill = append(plan.spill, Route{})
		copy(plan.spill[1:], plan.spill[:len(plan.spill)-1])
		plan.spill[0] = carried
	} else {
		spillIndex := position - len(plan.inline)
		if spillIndex < 0 || spillIndex > len(plan.spill) {
			return false
		}
		plan.spill = append(plan.spill, Route{})
		copy(plan.spill[spillIndex+1:], plan.spill[spillIndex:len(plan.spill)-1])
		plan.spill[spillIndex] = candidate
	}
	plan.count++
	return true
}

type SourceFact struct {
	fact      valuedomain.Value
	present   bool
	available bool
}

func DeriveCaptureRoutes(schema placementdomain.Schema, values *valuedomain.Schema, facts []SourceFact) (RoutePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return RoutePlan{}, false
	}
	// Collect every exact source directly into the plan's canonical inline
	// route set. The selected-read path is physical-order independent because
	// addRoute recovers dense Heap tag order while it unions each source.
	var plan RoutePlan
	for _, item := range facts {
		if !item.available || !values.Equal(item.fact, item.fact) {
			return RoutePlan{}, false
		}
		if !item.present {
			if !values.Equal(item.fact, values.Bottom()) {
				return RoutePlan{}, false
			}
			continue
		}
		widen, factOK := collectValueRoutes(schema, values, item.fact, &plan)
		if !factOK {
			return RoutePlan{}, false
		}
		if widen {
			plan, allOK := allAllocationPlan(schema)
			if !allOK {
				return RoutePlan{}, false
			}
			plan.class = routeWidened
			return plan, true
		}
	}
	if plan.RouteCount() == 0 {
		plan.class = routeScalar
		return plan, true
	}
	plan.class = routeExact
	return plan, true
}

// collectValueRoutes joins one exact Value fact directly into an owned route
// plan. It avoids a temporary index map and post-hoc sort on the selected-read
// hot path while retaining every owner/canonical-key check.
func collectValueRoutes(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value, plan *RoutePlan) (widen bool, ok bool) {
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
		if !denseOK || !plan.addRoute(Route{key: key, tag: RouteTag(uint64(dense) + 1)}) {
			return false, false
		}
	}
	return projection.HasOpaque(), true
}

// allAllocationPlan keeps widening lazy. It counts and authenticates the
// owner's allocation coordinates once, then routeAt/routeAtTag derive each
// selected route from that same immutable schema without copying a root
// catalogue.
func allAllocationPlan(schema placementdomain.Schema) (RoutePlan, bool) {
	if !schema.Valid() {
		return RoutePlan{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount < 0 {
		return RoutePlan{}, false
	}
	plan := RoutePlan{
		allRoot:          true,
		allRootPrefix:    true,
		allRootSchema:    schema,
		allRootDenseSize: denseCount,
	}
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return RoutePlan{}, false
		}
		if key.Kind() != heapdomain.RootAllocation {
			plan.allRootPrefix = false
			continue
		}
		plan.count++
	}
	return plan, true
}

func RouteAtTag(plan RoutePlan, tag RouteTag) (Route, bool) {
	if plan.allRoot {
		return plan.allRootAtTag(tag)
	}
	low, high := 0, plan.RouteCount()
	for low < high {
		middle := low + (high-low)/2
		candidate, candidateOK := plan.RouteAt(middle)
		if !candidateOK {
			return Route{}, false
		}
		if candidate.tag < tag {
			low = middle + 1
		} else {
			high = middle
		}
	}
	candidate, candidateOK := plan.RouteAt(low)
	if candidateOK && candidate.tag == tag {
		return candidate, true
	}
	return Route{}, false
}

// oneDeliveredCell reads the single coordinate an exact delivery carries. A
// delivery of any other width answers no cell at all: a rule that reads one
// coordinate has no fold to run over two.
func oneDeliveredCell[T any](count int, at func(int) (T, bool, bool)) (value T, present, available bool) {
	if at == nil || count != 1 {
		return value, false, false
	}
	return at(0)
}
