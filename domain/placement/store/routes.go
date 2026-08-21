package store

import (
	"sort"

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
	class  routeClass
	routes []Route
}

func (plan RoutePlan) Valid() bool     { return plan.class != routeInvalid }
func (plan RoutePlan) Bottom() bool    { return plan.class == routeBottom }
func (plan RoutePlan) Widened() bool   { return plan.class == routeWidened }
func (plan RoutePlan) RouteCount() int { return len(plan.routes) }

func (plan RoutePlan) RouteAt(index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	return plan.routes[index], true
}

// Plan derives exact or conservative routes from the existing Value fact.
// Exact allocation references select their own Heap roots. Top and opaque
// alternatives widen to every Placement allocation root. Exact non-allocation
// roots (including Boot handles) and scalars produce no local route.
func Plan(schema placement.Schema, values *valuedomain.Schema, fact valuedomain.Value) (RoutePlan, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return RoutePlan{}, false
	}
	if fact.IsBottom() {
		return RoutePlan{class: routeBottom}, true
	}
	if fact.IsTop() {
		routes, ok := allAllocationRoutes(schema)
		if !ok {
			return RoutePlan{}, false
		}
		return RoutePlan{class: routeWidened, routes: routes}, true
	}
	// Pull exact alternatives directly into the route buffer. Sorting and
	// compacting that buffer retains Heap order and alias deduplication without
	// materializing Value.Atoms or a callback closure for every relation.
	var routes []Route
	widen := false
	heapSchema := schema.Heap()
	if !values.Equal(fact, fact) {
		return RoutePlan{}, false
	}
	validAtoms := true
atoms:
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			validAtoms = false
			break
		}
		classification, classificationOK := placement.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			validAtoms = false
			break atoms
		}
		switch classification.Class {
		case placement.AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
				validAtoms = false
				break atoms
			}
			index, indexOK := heapSchema.KeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(index)
			if !indexOK || index < 0 || !canonicalOK || canonical != key {
				validAtoms = false
				break atoms
			}
			routes = append(routes, Route{Key: key, Tag: uint64(index) + 1})
		case placement.AtomClassOpaque:
			widen = true
		}
	}
	if !validAtoms {
		return RoutePlan{}, false
	}
	if widen {
		allRoutes, ok := allAllocationRoutes(schema)
		if !ok {
			return RoutePlan{}, false
		}
		return RoutePlan{class: routeWidened, routes: allRoutes}, true
	}
	if len(routes) == 0 {
		return RoutePlan{class: routeScalar}, true
	}
	sort.Sort(routesByTag(routes))
	unique := 0
	for index := range routes {
		candidate := routes[index]
		if unique != 0 && routes[unique-1].Tag == candidate.Tag {
			continue
		}
		routes[unique] = candidate
		unique++
	}
	routes = routes[:unique]
	return RoutePlan{class: routeExact, routes: routes}, true
}

func allAllocationRoutes(schema placement.Schema) ([]Route, bool) {
	if !schema.Valid() {
		return nil, false
	}
	denseCount := schema.DenseKeyCount()
	routes := make([]Route, 0, denseCount)
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return nil, false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		routes = append(routes, Route{Key: key, Tag: uint64(dense) + 1})
	}
	return routes, true
}

func (plan RoutePlan) routeAtTag(tag uint64) (Route, bool) {
	low, high := 0, len(plan.routes)
	for low < high {
		middle := low + (high-low)/2
		if plan.routes[middle].Tag < tag {
			low = middle + 1
			continue
		}
		high = middle
	}
	index := low
	if index < len(plan.routes) && plan.routes[index].Tag == tag {
		return plan.routes[index], true
	}
	return Route{}, false
}

type routesByTag []Route

func (routes routesByTag) Len() int           { return len(routes) }
func (routes routesByTag) Less(i, j int) bool { return routes[i].Tag < routes[j].Tag }
func (routes routesByTag) Swap(i, j int)      { routes[i], routes[j] = routes[j], routes[i] }
