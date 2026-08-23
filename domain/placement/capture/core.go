package capture

import (
	"github.com/wippyai/go-lua/analysis/engine"
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
type routeTag uint64

type source struct {
	module     identity.ContentID
	id         identity.ContentID
	coordinate valuedomain.Coordinate
	tag        routeTag
}

// operand is one closure allocation root together with the canonical Value
// coordinates named by its Program capture rows. Every source coordinate is
// authenticated before the operand is admitted; there is no unresolved or
// compensating operand state.
type operand struct {
	key        heapdomain.Key
	id         identity.ContentID
	coordinate uint64
	sources    []source
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

func sourceCanonical(values *valuedomain.Schema, candidate source) (source, bool) {
	if values == nil || !values.Valid() || !candidate.module.Available() || !candidate.id.Available() || candidate.tag == 0 {
		return source{}, false
	}
	coordinate, coordinateOK := values.CoordinateForMountedSemantic(candidate.module, candidate.id)
	if !coordinateOK || coordinate != candidate.coordinate {
		return source{}, false
	}
	if _, indexOK := values.CoordinateIndex(candidate.coordinate); !indexOK {
		return source{}, false
	}
	return candidate, true
}

// sourceOrdinal recovers the authored capture position from a selected route
// tag. The engine exposes Selection rows in physical Unit order, so the
// selection ordinal is not the capture ordinal. A source tag is one-based and
// must name exactly one declared source.
func sourceOrdinal(candidate operand, tag routeTag) (int, bool) {
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
	var seenTags map[routeTag]struct{}
	if len(candidate.sources) > 1 {
		seenTags = make(map[routeTag]struct{}, len(candidate.sources))
	}
	var previousTag routeTag
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
	module, programID, allocationID, kind, _, originOK := schema.Heap().AllocationOriginForKey(canonical.key)
	if !originOK || kind != heapdomain.AllocationClosure || !module.Available() || !programID.Available() || !allocationID.Available() {
		return operand{}, false, false
	}
	candidate = canonical
	mount, mountOK := schema.Heap().MountedArtifactForModule(module)
	if !mountOK || mount.ProgramID != programID || mount.Snapshot == nil {
		return operand{}, false, false
	}
	program := mount.Snapshot.Program()
	if !program.Available() {
		return operand{}, false, false
	}
	state, stateOK := program.ColdState()
	targets, targetsOK := calltarget.NewView(state)
	allocations, allocationsOK := heapallocation.NewView(state)
	target, targetOK := targets.ForAllocation(allocationID)
	allocation, allocationOK := allocations.AllocationForID(allocationID)
	boundary, boundaryOK := program.FunctionBoundaryForBody(target.BodyID())
	if !stateOK || !targetsOK || !allocationsOK || !targetOK || !allocationOK || allocation.Role() != heapallocation.RoleClosure || !boundaryOK {
		return operand{}, false, false
	}
	count := boundary.CaptureCount()
	if count == 0 {
		return operand{}, false, true
	}
	offset, spanCount, spanOK := boundary.CaptureSpan()
	if !spanOK || int(spanCount) != count || uint64(offset)+uint64(spanCount) > uint64(^uint32(0)) {
		return operand{}, false, false
	}
	candidate.sources = make([]source, 0, count)
	for index := 0; index < count; index++ {
		capture, captureOK := program.FunctionCaptureAt(int(offset) + index)
		position, positionOK := capture.Position()
		if !captureOK || !positionOK || position != index || capture.InnerBodyID() != boundary.BodyID() {
			return operand{}, false, false
		}
		storageID := capture.OuterStorageCellID()
		if !storageID.Available() {
			return operand{}, false, false
		}
		coordinate, coordinateOK := values.CoordinateForMountedSemantic(module, storageID)
		item := source{module: module, id: storageID, coordinate: coordinate, tag: routeTag(uint64(index) + 1)}
		if !coordinateOK || !coordinate.Valid() {
			return operand{}, false, false
		}
		candidate.sources = append(candidate.sources, item)
	}
	if len(candidate.sources) != count {
		return operand{}, false, false
	}
	return candidate, true, true
}

type route struct {
	key heapdomain.Key
	tag routeTag
}

type routeClass uint8

const (
	routeBottom routeClass = iota
	routeExact
	routeScalar
	routeWidened
)

type routePlan struct {
	class  routeClass
	inline [captureRouteInlineCapacity]route
	spill  []route
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

func (plan routePlan) routeCount() int {
	if plan.count < 0 {
		return 0
	}
	return plan.count
}

func (plan routePlan) routeAt(index int) (route, bool) {
	if index < 0 || index >= plan.routeCount() {
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
	if !plan.allRoot || index < 0 || index >= plan.count || !plan.allRootSchema.Valid() {
		return route{}, false
	}
	if plan.allRootPrefix {
		key, keyOK := plan.allRootSchema.KeyAt(index)
		return route{key: key, tag: routeTag(uint64(index) + 1)}, keyOK && key.Kind() == heapdomain.RootAllocation
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

type sourceFact struct {
	fact      valuedomain.Value
	present   bool
	available bool
}

func routePlanForFacts(schema placementdomain.Schema, values *valuedomain.Schema, facts []sourceFact) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	// Collect every exact source directly into the plan's canonical inline
	// route set. The selected-read path is physical-order independent because
	// addRoute recovers dense Heap tag order while it unions each source.
	var plan routePlan
	for _, item := range facts {
		if !item.available || !values.Equal(item.fact, item.fact) {
			return routePlan{}, false
		}
		if !item.present {
			if !values.Equal(item.fact, values.Bottom()) {
				return routePlan{}, false
			}
			continue
		}
		widen, factOK := collectValueRoutes(schema, values, item.fact, &plan)
		if !factOK {
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
	if plan.routeCount() == 0 {
		plan.class = routeScalar
		return plan, true
	}
	plan.class = routeExact
	return plan, true
}

// collectValueRoutes joins one exact Value fact directly into an owned route
// plan. It avoids a temporary index map and post-hoc sort on the selected-read
// hot path while retaining every owner/canonical-key check.
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
		if key.Kind() != heapdomain.RootAllocation {
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
	candidate, candidateOK := plan.routeAt(low)
	if candidateOK && candidate.tag == tag {
		return candidate, true
	}
	return route{}, false
}

func oneOrderedCell[T any](cells engine.OrderedCells[T]) (value T, present, available bool) {
	if cells.Count() != 1 {
		return value, false, false
	}
	return cells.At(0)
}

func captureValue(parent, current placementdomain.Fact) (placementdomain.Fact, bool) {
	return placementdomain.ThroughContainerChecked(current, parent)
}
