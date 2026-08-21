package capture

import (
	"sort"

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
	index, indexOK := schema.Heap().KeyIndex(key)
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
	mount, mountOK := schema.Heap().ArtifactMountForModule(module)
	if !mountOK || mount.ProgramID() != programID || mount.Snapshot() == nil {
		return operand{}, false, false
	}
	program := mount.Snapshot().Program()
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
	routes []route
}

type sourceFact struct {
	fact      valuedomain.Value
	present   bool
	available bool
}

func routePlanForValue(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	if values.Equal(fact, values.Bottom()) {
		return routePlan{class: routeBottom}, true
	}
	if values.Equal(fact, values.Top()) {
		routes, ok := allAllocationRoutes(schema)
		if !ok {
			return routePlan{}, false
		}
		return routePlan{class: routeWidened, routes: routes}, true
	}
	indexes := make(map[int]struct{})
	widen, atomsOK := collectValueIndexes(schema, values, fact, indexes)
	if !atomsOK {
		return routePlan{}, false
	}
	if widen {
		routes, ok := allAllocationRoutes(schema)
		if !ok {
			return routePlan{}, false
		}
		return routePlan{class: routeWidened, routes: routes}, true
	}
	if len(indexes) == 0 {
		return routePlan{class: routeScalar}, true
	}
	ordered := make([]int, 0, len(indexes))
	for index := range indexes {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	routes := make([]route, 0, len(ordered))
	for _, index := range ordered {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			return routePlan{}, false
		}
		routes = append(routes, route{key: key, tag: routeTag(uint64(index) + 1)})
	}
	return routePlan{class: routeExact, routes: routes}, true
}

func routePlanForFacts(schema placementdomain.Schema, values *valuedomain.Schema, facts []sourceFact) (routePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return routePlan{}, false
	}
	// Collect every exact source into one index set. The previous composition
	// called routePlanForValue for every source, allocating and sorting an
	// intermediate plan each time before unioning those plans. Capture source
	// counts are small, but this sits on the selected-read transfer path and
	// the repeated work became quadratic in the number of captured sources.
	indexes := make(map[int]struct{}, len(facts))
	for _, item := range facts {
		if !item.available || !item.present {
			return routePlan{}, false
		}
		widen, factOK := collectValueIndexes(schema, values, item.fact, indexes)
		if !factOK {
			return routePlan{}, false
		}
		if widen {
			routes, allOK := allAllocationRoutes(schema)
			if !allOK {
				return routePlan{}, false
			}
			return routePlan{class: routeWidened, routes: routes}, true
		}
	}
	if len(indexes) == 0 {
		return routePlan{class: routeScalar}, true
	}
	ordered := make([]int, 0, len(indexes))
	for index := range indexes {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	routes := make([]route, 0, len(ordered))
	for _, index := range ordered {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			return routePlan{}, false
		}
		routes = append(routes, route{key: key, tag: routeTag(uint64(index) + 1)})
	}
	return routePlan{class: routeExact, routes: routes}, true
}

// collectValueIndexes joins one exact Value fact directly into the caller's
// index set. It uses the pull-based Value atom accessors so the selected-read
// path does not materialize a temporary atom slice or callback closure for
// every captured source.
func collectValueIndexes(schema placementdomain.Schema, values *valuedomain.Schema, fact valuedomain.Value, indexes map[int]struct{}) (widen bool, ok bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) || indexes == nil {
		return false, false
	}
	if values.Equal(fact, values.Bottom()) {
		return false, true
	}
	if values.Equal(fact, values.Top()) {
		return true, true
	}
	if !values.Equal(fact, fact) {
		return false, false
	}
	heapSchema := schema.Heap()
	valid := true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			valid = false
			break
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			valid = false
			break atoms
		}
		switch classification.Class {
		case placementdomain.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
				valid = false
				break atoms
			}
			index, indexOK := heapSchema.KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || index < 0 || !canonicalOK || canonical != key {
				valid = false
				break atoms
			}
			indexes[index] = struct{}{}
		case placementdomain.AtomClassOpaque:
			widen = true
		}
	}
	return widen, valid
}

func allAllocationRoutes(schema placementdomain.Schema) ([]route, bool) {
	if !schema.Valid() {
		return nil, false
	}
	denseCount := schema.DenseKeyCount()
	routes := make([]route, 0, denseCount)
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return nil, false
		}
		if key.Kind() == heapdomain.RootAllocation {
			routes = append(routes, route{key: key, tag: routeTag(uint64(dense) + 1)})
		}
	}
	return routes, true
}

func routeAtTag(plan routePlan, tag routeTag) (route, bool) {
	index := sort.Search(len(plan.routes), func(index int) bool { return plan.routes[index].tag >= tag })
	if index < len(plan.routes) && plan.routes[index].tag == tag {
		return plan.routes[index], true
	}
	return route{}, false
}

func validPlacement(fact placementdomain.Placement) bool {
	switch fact {
	case placementdomain.Bottom, placementdomain.Stack, placementdomain.OwnedHeap, placementdomain.SharedHeap, placementdomain.Unknown:
		return true
	default:
		return false
	}
}

func oneOrderedCell[T any](cells engine.OrderedCells[T]) (value T, present, available bool) {
	if cells.Count() != 1 {
		return value, false, false
	}
	return cells.At(0)
}

func captureValue(parent, current placementdomain.Placement) placementdomain.Placement {
	return placementdomain.Join(parent, current)
}
