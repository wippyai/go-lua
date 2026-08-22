package store

import (
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one exact Placement route selected from a Value reference. Tag is
// the one-based dense Heap coordinate carried by the engine selection; Key is
// the owner-issued Heap root identity.
type Route struct {
	Key heap.Key
	Tag uint64
}

type routeClass uint8

const (
	routeInvalid routeClass = iota
	routeBottom
	routeExact
	routeScalar
	routeWidened
)

// RoutePlan is the closed routing result for one Value fact. A scalar has no
// Placement route; Bottom is kept distinct so a consumer cannot confuse a
// missing Value with an ordinary scalar.
type RoutePlan struct {
	class routeClass

	// Exact plans keep their common, small route set in the plan value. The
	// spill suffix is explicit and invocation-local; it is only used when an
	// exact Value contains more routes than the bounded inline prefix. Keeping
	// the prefix by value means the normal one/two-route store transfer never
	// allocates a slice just to return its plan.
	inline [routeInlineWidth]Route
	spill  []Route
	count  int

	// A widened plan is a view over the owner's canonical dense coordinates.
	// It deliberately does not retain a copied allocation-root catalogue. The
	// owner schema is immutable and therefore safe to carry across concurrent
	// rule invocations.
	allRoot          bool
	allRootPrefix    bool
	allRootSchema    placement.Schema
	allRootDenseSize int
}

const routeInlineWidth = 8

func (plan RoutePlan) Valid() bool   { return plan.class != routeInvalid }
func (plan RoutePlan) Bottom() bool  { return plan.class == routeBottom }
func (plan RoutePlan) Widened() bool { return plan.class == routeWidened }
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
	spill := index - len(plan.inline)
	if spill < 0 || spill >= len(plan.spill) {
		return Route{}, false
	}
	return plan.spill[spill], true
}

// Plan derives exact or conservative routes from the existing Value fact.
// Exact allocation references select their own Heap roots. Top and opaque
// alternatives widen to every Placement allocation root. Exact non-allocation
// roots (including Boot handles) and scalars produce no local route.
func Plan(schema placement.Schema, values *valuedomain.Schema, fact valuedomain.Value) (RoutePlan, bool) {
	projection, projectionOK := placement.ProjectValueAllocations(schema, values, fact)
	if !projectionOK {
		return RoutePlan{}, false
	}
	if projection.IsBottom() {
		return RoutePlan{class: routeBottom}, true
	}
	if projection.IsTop() {
		plan, ok := allAllocationPlan(schema)
		if !ok {
			return RoutePlan{}, false
		}
		plan.class = routeWidened
		return plan, true
	}
	// Pull exact alternatives directly into bounded dense-coordinate scratch.
	// Insertion keeps the route set canonical while it is built, so no sort,
	// map, or post-hoc copied root catalogue is needed.
	var plan RoutePlan
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		if !keyOK {
			return RoutePlan{}, false
		}
		dense, denseOK := schema.Heap().AllocationKeyIndex(key)
		if !denseOK || !plan.addExact(Route{Key: key, Tag: uint64(dense) + 1}) {
			return RoutePlan{}, false
		}
	}
	if projection.HasOpaque() {
		widened, ok := allAllocationPlan(schema)
		if !ok {
			return RoutePlan{}, false
		}
		widened.class = routeWidened
		return widened, true
	}
	if plan.count == 0 {
		plan.class = routeScalar
		return plan, true
	}
	plan.class = routeExact
	return plan, true
}

// allAllocationPlan keeps widening lazy. It counts and authenticates the
// owner's allocation coordinates once, then RouteAt/routeAtTag derive each
// selected route from that same schema. Heap seals Program and fresh
// allocation roots before Boot roots, so the common representation is a
// direct dense prefix; the defensive non-prefix form remains allocation-free
// and scans the owner when indexed.
func allAllocationPlan(schema placement.Schema) (RoutePlan, bool) {
	if !schema.Valid() {
		return RoutePlan{}, false
	}
	denseCount := schema.DenseKeyCount()
	if denseCount < 0 {
		return RoutePlan{}, false
	}
	plan := RoutePlan{
		allRoot:          true,
		allRootSchema:    schema,
		allRootDenseSize: denseCount,
		allRootPrefix:    true,
	}
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return RoutePlan{}, false
		}
		if key.Kind() != heap.RootAllocation {
			plan.allRootPrefix = false
			continue
		}
		plan.count++
	}
	return plan, true
}

func (plan RoutePlan) routeAtTag(tag uint64) (Route, bool) {
	if plan.allRoot {
		return plan.allRootAtTag(tag)
	}
	count := plan.RouteCount()
	low, high := 0, count
	for low < high {
		middle := low + (high-low)/2
		candidate, candidateOK := plan.RouteAt(middle)
		if !candidateOK {
			return Route{}, false
		}
		if candidate.Tag < tag {
			low = middle + 1
			continue
		}
		high = middle
	}
	index := low
	if index < count {
		candidate, candidateOK := plan.RouteAt(index)
		if candidateOK && candidate.Tag == tag {
			return candidate, true
		}
	}
	return Route{}, false
}

func (plan RoutePlan) allRootAt(index int) (Route, bool) {
	if !plan.allRoot || index < 0 || index >= plan.count || plan.allRootDenseSize < 0 || !plan.allRootSchema.Valid() {
		return Route{}, false
	}
	if plan.allRootPrefix {
		key, keyOK := plan.allRootSchema.KeyAt(index)
		return Route{Key: key, Tag: uint64(index) + 1}, keyOK && key.Kind() == heap.RootAllocation
	}
	ordinal := 0
	for dense := 0; dense < plan.allRootDenseSize; dense++ {
		key, keyOK := plan.allRootSchema.KeyAt(dense)
		if !keyOK {
			return Route{}, false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		if ordinal == index {
			return Route{Key: key, Tag: uint64(dense) + 1}, true
		}
		ordinal++
	}
	return Route{}, false
}

func (plan RoutePlan) allRootAtTag(tag uint64) (Route, bool) {
	if !plan.allRoot || tag == 0 || plan.allRootDenseSize < 0 || !plan.allRootSchema.Valid() {
		return Route{}, false
	}
	dense := tag - 1
	if dense >= uint64(plan.allRootDenseSize) {
		return Route{}, false
	}
	key, keyOK := plan.allRootSchema.KeyAt(int(dense))
	if !keyOK || key.Kind() != heap.RootAllocation {
		return Route{}, false
	}
	return Route{Key: key, Tag: tag}, true
}

// addExact inserts one exact route in canonical dense-tag order and
// deduplicates aliases. The inline prefix is bounded; only routes beyond it
// use the explicit spill suffix.
func (plan *RoutePlan) addExact(candidate Route) bool {
	if plan == nil || plan.allRoot || plan.count < 0 || candidate.Tag == 0 {
		return false
	}
	position := 0
	for position < plan.count {
		current, currentOK := plan.RouteAt(position)
		if !currentOK {
			return false
		}
		switch {
		case current.Tag == candidate.Tag:
			return current.Key == candidate.Key
		case current.Tag > candidate.Tag:
			goto insert
		default:
			position++
		}
	}

insert:
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
		spill := position - len(plan.inline)
		if spill < 0 || spill > len(plan.spill) {
			return false
		}
		plan.spill = append(plan.spill, Route{})
		copy(plan.spill[spill+1:], plan.spill[spill:len(plan.spill)-1])
		plan.spill[spill] = candidate
	}
	plan.count++
	return true
}
