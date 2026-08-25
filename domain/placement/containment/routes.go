package containment

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// Route is one parent-to-child containment route: the child cell the rule
// reads and publishes into, and the tag that names which parent selected it.
// A containment route moves nothing between cells - the parent's placement is
// joined into the child's own cell - so both endpoints are that one key.
type Route struct {
	key heapdomain.Key
	tag uint64
}

// Coordinates are the cell this route reads and the cell it publishes into.
func (item Route) Coordinates() (heapdomain.Key, heapdomain.Key) { return item.key, item.key }

// Predicate is the tag the selection stamps this row with: the lossless
// parent/child coordinate pair, which is what correlates a returned cell with
// the parent whose containment selected it.
func (item Route) Predicate() uint64 { return item.tag }

// RoutePlan is the route set one containment invocation selects, in the order
// the parent summary is walked in.
type RoutePlan struct {
	routes []Route
}

// RouteCount is the census of the selected route set.
func (plan RoutePlan) RouteCount() int { return len(plan.routes) }

// RouteAt returns one route at its selection position.
func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	return plan.routes[index], true
}

// DeriveContainmentRoutes is the operation that publishes the containment
// route rows: for every authenticated parent placement, the root allocations
// its Heap value contains, widened to every root when that value is Top or
// carries an opaque containment edge.
//
// Which rows exist depends on the two complete vectors read before it, so they
// are produced rather than enumerated, and this takes those vectors as its
// sealed inputs. It is the same walk the hand rule performed against the live
// selector; the selector now receives the rows it returns.
func DeriveContainmentRoutes(schema placement.Schema, heapSchema heapdomain.Schema, placements engine.OrderedCells[placement.Fact], heaps engine.OrderedCells[heapdomain.Value]) (RoutePlan, bool) {
	if !schema.Valid() || !heapSchema.Valid() || schema.Heap() != heapSchema {
		return RoutePlan{}, false
	}
	var plan RoutePlan
	for parentIndex := 0; parentIndex < placements.Count(); parentIndex++ {
		parent, parentPresent, parentAvailable := placements.At(parentIndex)
		parent, parentOK := placement.AuthenticateFactCell(parent, parentPresent, parentAvailable)
		parentKey, parentKeyOK := schema.KeyAt(parentIndex)
		heapIndex, heapIndexOK := heapSchema.AllocationKeyIndex(parentKey)
		heapValue, heapOK := summaryCell(heaps, heapIndex, heapSchema.Bottom(), heapdomain.Equal)
		if !parentOK || !parentKeyOK || !heapIndexOK || !heapOK || !validPlacement(parent) || !heapValue.Valid() {
			return RoutePlan{}, false
		}
		if heapdomain.Equal(heapValue, heapSchema.Bottom()) {
			continue
		}
		emit := func(child heapdomain.Key) bool {
			childIndex, childOK := heapSchema.AllocationKeyIndex(child)
			if !childOK || childIndex < 0 || childIndex >= placements.Count() || child.Kind() != heapdomain.RootAllocation {
				return false
			}
			tag, tagOK := routeTag(parentIndex, childIndex)
			if !tagOK {
				return false
			}
			plan.routes = append(plan.routes, Route{key: child, tag: tag})
			return true
		}
		if heapdomain.Equal(heapValue, heapSchema.Top()) {
			if !walkAllRoots(schema, heapSchema, emit) {
				return RoutePlan{}, false
			}
			continue
		}
		opaque, complete := containmentEvidence(heapSchema, heapValue)
		if !complete {
			return RoutePlan{}, false
		}
		if opaque {
			if !walkAllRoots(schema, heapSchema, emit) {
				return RoutePlan{}, false
			}
			continue
		}
		if !walkContainments(heapSchema, heapValue, emit) {
			return RoutePlan{}, false
		}
	}
	return plan, true
}

func walkAllRoots(schema placement.Schema, heapSchema heapdomain.Schema, emit func(heapdomain.Key) bool) bool {
	if emit == nil || !schema.Valid() || schema.Heap() != heapSchema {
		return false
	}
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK || !key.Valid() {
			return false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if !heapSchema.OwnsKey(key) {
			return false
		}
		if !emit(key) {
			return false
		}
	}
	return true
}

func containmentEvidence(heapSchema heapdomain.Schema, value heapdomain.Value) (opaque, complete bool) {
	if !heapSchema.Valid() || !value.Valid() || value.IsTop() {
		return false, false
	}
	complete = heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if !observation.Valid() {
			return false
		}
		switch observation.Kind() {
		case heapdomain.ContainmentNone:
			return true
		case heapdomain.ContainmentUnknown:
			opaque = true
			return true
		case heapdomain.ContainmentExact:
			reference, referenceOK := observation.Reference()
			child, _, childOK := reference.Key()
			return referenceOK && childOK && heapSchema.OwnsKey(child)
		default:
			return false
		}
	})
	return opaque, complete
}

func walkContainments(heapSchema heapdomain.Schema, value heapdomain.Value, emit func(heapdomain.Key) bool) bool {
	if !heapSchema.Valid() || !value.Valid() || value.IsTop() || emit == nil {
		return false
	}
	return heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if !observation.Valid() {
			return false
		}
		switch observation.Kind() {
		case heapdomain.ContainmentNone:
			return true
		case heapdomain.ContainmentExact:
			reference, referenceOK := observation.Reference()
			child, _, childOK := reference.Key()
			if !referenceOK || !childOK || !heapSchema.OwnsKey(child) {
				return false
			}
			return child.Kind() != heapdomain.RootAllocation || emit(child)
		default:
			return false
		}
	})
}
