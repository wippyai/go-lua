// Package recentplan owns the bounded canonical route-set reduction used by
// Heap consumers that can authorize an exact Recent allocation root.
//
// A plan keeps its ordinary prefix inline so the common one-to-eight-root path
// stays allocation-free. Wider declarations use an invocation-local suffix.
// Routes are kept in RawRouteTag order. Adding an existing tag is an alias
// only when it names the same Heap key; a tag collision with another key is
// malformed owner state and fails closed. Intersection preserves that same
// invariant.
package recentplan

import "github.com/wippyai/go-lua/domain/heap"

// InlineWidth is the fixed route prefix used by ordinary Heap consumers.
const InlineWidth = 8

// Route is one exact Heap route. Key and Tag are issued by Heap's schema; the
// package does not derive or reinterpret either authority.
type Route struct {
	Key heap.Key
	Tag heap.RawRouteTag
}

// Plan is a sorted, duplicate-free set of exact Heap routes. The overflow
// suffix is only allocated when a declaration exceeds InlineWidth routes.
type Plan struct {
	inline [InlineWidth]Route
	extra  []Route
	size   int
}

// Count reports the number of routes, treating malformed negative state as an
// empty plan.
func (plan Plan) Count() int {
	if plan.size < 0 {
		return 0
	}
	return plan.size
}

// At returns the route at its canonical tag-sorted position.
func (plan Plan) At(index int) (Route, bool) {
	if index < 0 || index >= plan.size {
		return Route{}, false
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	extra := index - len(plan.inline)
	if extra < 0 || extra >= len(plan.extra) {
		return Route{}, false
	}
	return plan.extra[extra], true
}

// Add inserts one route in canonical RawRouteTag order. Repeated tags are
// aliases only when they carry the same key; a collision fails closed.
// RawRouteTag zero is left to the caller's schema validation so this utility
// preserves the historical formal-freeze behavior. Publication-freeze keeps
// its existing explicit nonzero-tag guard before calling Add.
func (plan *Plan) Add(candidate Route) bool {
	if plan == nil || plan.size < 0 {
		return false
	}
	position := 0
	for position < plan.size {
		current, ok := plan.At(position)
		if !ok {
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
	if plan.size < len(plan.inline) {
		for index := plan.size; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.size++
		return true
	}

	// Keep the first InlineWidth routes in the fixed array and the remainder in
	// an overflow suffix. Inserting before the boundary carries the old inline
	// tail into that suffix; no second complete route slice is created.
	if position < len(plan.inline) {
		carried := plan.inline[len(plan.inline)-1]
		for index := len(plan.inline) - 1; index > position; index-- {
			plan.inline[index] = plan.inline[index-1]
		}
		plan.inline[position] = candidate
		plan.extra = append(plan.extra, Route{})
		copy(plan.extra[1:], plan.extra[:len(plan.extra)-1])
		plan.extra[0] = carried
	} else {
		extra := position - len(plan.inline)
		if extra < 0 || extra > len(plan.extra) {
			return false
		}
		plan.extra = append(plan.extra, Route{})
		copy(plan.extra[extra+1:], plan.extra[extra:len(plan.extra)-1])
		plan.extra[extra] = candidate
	}
	plan.size++
	return true
}

// Intersection returns the exact route intersection in canonical tag order.
// A shared tag naming different keys is malformed state and fails closed.
func (plan Plan) Intersection(other Plan) (Plan, bool) {
	if plan.size < 0 || other.size < 0 {
		return Plan{}, false
	}
	var result Plan
	left, right := 0, 0
	for left < plan.size && right < other.size {
		leftRoute, leftOK := plan.At(left)
		rightRoute, rightOK := other.At(right)
		if !leftOK || !rightOK {
			return Plan{}, false
		}
		switch {
		case leftRoute.Tag < rightRoute.Tag:
			left++
		case rightRoute.Tag < leftRoute.Tag:
			right++
		default:
			if leftRoute.Key != rightRoute.Key || !result.Add(leftRoute) {
				return Plan{}, false
			}
			left++
			right++
		}
	}
	return result, true
}

// RouteForTag performs the ordered lookup used by Heap's routed reducer. Plans
// are kept sorted by RawRouteTag, so the common lookup is logarithmic rather
// than rescanning every route on each routed cell.
func RouteForTag(plan Plan, tag heap.RawRouteTag) (Route, bool) {
	count := plan.Count()
	left, right := 0, count
	for left < right {
		middle := left + (right-left)/2
		candidate, ok := plan.At(middle)
		if !ok {
			return Route{}, false
		}
		if candidate.Tag < tag {
			left = middle + 1
			continue
		}
		right = middle
	}
	if left >= count {
		return Route{}, false
	}
	candidate, ok := plan.At(left)
	return candidate, ok && candidate.Tag == tag
}
