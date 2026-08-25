// Package returnroute derives the Value coordinates one mounted call site's
// executable bodies publish their first return value at.
//
// It is the declared relation the body-result rule selects over: the members
// are the canonical first return members of the bodies THIS call may reach, in
// the coordinate order the Value schema numbers its cells by, and the tag a
// member carries is that coordinate's own dense index raised by one. Nothing
// here reads a Factor cell - which bodies a call reaches is Call's cold
// answer, and what each return member IS is the cell the selection observes at
// the member this derivation named.
package returnroute

import (
	"sort"

	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one body return this call reaches: the Value coordinate the first
// return member is observed at, and the tag it is correlated by. The tag is
// that coordinate's dense index raised by one, so zero stays the absent tag.
type Route struct {
	coordinate valuedomain.Coordinate
	tag        uint64
	set        bool
}

// Coordinate is the Value coordinate this member's cell is observed at.
func (route Route) Coordinate() (valuedomain.Coordinate, bool) { return route.coordinate, route.set }

// Predicate is the owner-issued tag this member is correlated by.
func (route Route) Predicate() (uint64, bool) { return route.tag, route.set }

// Plan is one call site's whole ordered member set.
type Plan struct{ routes []Route }

// Count is the member census this selection spans.
func Count(plan Plan) int { return len(plan.routes) }

// At projects one member in the canonical order Derive fixed.
func At(plan Plan, index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	return plan.routes[index], true
}

// Selection is one call site's whole reading of the bodies it dispatches to:
// the tags of the first return members those bodies publish, whether any body
// was reached at all, whether some reached return carries no value, and
// whether the evidence is beyond enumeration.
type Selection struct {
	tags    []uint64
	hasBody bool
	nilCase bool
	top     bool
}

// Top reports that the returned value is beyond enumeration.
func (selection Selection) Top() bool { return selection.top }

// HasBody reports that the call reaches at least one executable body. A site
// that reaches none carries no evidence of this kind.
func (selection Selection) HasBody() bool { return selection.hasBody }

// NilCase reports that some reached return path publishes no first value, so
// the call's first result is nil along it.
func (selection Selection) NilCase() bool { return selection.nilCase }

// Tags is the strictly ascending set of member tags this site observes.
func (selection Selection) Tags() []uint64 { return selection.tags }

// Contains reports whether one observed tag belongs to the set this site
// named.
func (selection Selection) Contains(tag uint64) bool {
	index := sort.Search(len(selection.tags), func(index int) bool { return selection.tags[index] >= tag })
	return index < len(selection.tags) && selection.tags[index] == tag
}

// BodyBoundaries answers the canonical return boundaries of one executable
// body.
//
// A callable body with no authored ReturnBoundary falls off its end and
// returns no values. The empty set is that exact Lua case; it is not missing
// analysis evidence. A boundary that names another body, or one this Value
// schema did not issue, is malformed geometry and is refused.
func BodyBoundaries(values *valuedomain.Schema, body calldomain.Body) ([]valuedomain.ReturnBoundary, bool) {
	if values == nil || !values.Valid() {
		return nil, false
	}
	module, moduleOK := body.ModuleKey()
	path, pathOK := body.BodyPath()
	if !moduleOK || !pathOK {
		return nil, false
	}
	boundaries, boundariesOK := values.ReturnBoundariesForBody(module, path)
	if !boundariesOK {
		boundaries = nil
	}
	for _, boundary := range boundaries {
		owner, ownerOK := boundary.BodyID()
		if !ownerOK || owner != path || !values.OwnsReturnBoundary(boundary) {
			return nil, false
		}
	}
	return boundaries, true
}

// Select validates one exact Call fact and answers the canonical first return
// members of every executable body that fact dispatches to.
//
// A Top or opaque call value reaches bodies this read did not observe, so the
// answer is beyond enumeration. A seed target belongs to the result-alias rule
// and is skipped here. A reached return with no fixed member publishes nil
// when it has no tail and is beyond enumeration when it has one, because a
// runtime suffix carries values with no fixed coordinate.
func Select(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate valuedomain.MountedCallResultSlot,
	fact calldomain.Value,
) (Selection, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() ||
		!values.LinkOwner().Matches(calls.LinkOwner()) ||
		!values.OwnsMountedCallResultSlot(candidate) || !callValueValid(fact) {
		return Selection{}, false
	}
	// This rule is the sole transfer for the fixed first return position. A
	// later result slot has its own semantic owner; accepting it here would
	// give the result-zero rule a second, incompatible output authority.
	ordinal, ordinalOK := candidate.Ordinal()
	if !ordinalOK || ordinal != 0 {
		return Selection{}, false
	}
	if fact.IsTop() || fact.HasOpaqueAlternative() {
		return Selection{top: true}, true
	}
	selection := Selection{}
	seen := make(map[uint64]struct{})
	for index := 0; index < fact.KnownTargetCount(); index++ {
		target, targetOK := fact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return Selection{}, false
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			continue
		}
		boundaries, boundariesOK := BodyBoundaries(values, body)
		if !boundariesOK {
			return Selection{}, false
		}
		selection.hasBody = true
		if len(boundaries) == 0 {
			selection.nilCase = true
			continue
		}
		for _, boundary := range boundaries {
			if boundary.MemberCount() == 0 {
				if boundary.HasTail() {
					selection.top = true
				} else {
					selection.nilCase = true
				}
				continue
			}
			member, memberOK := boundary.MemberAt(0)
			coordinate, coordinateOK := member.Coordinate()
			coordinateIndex, indexOK := values.CoordinateIndex(coordinate)
			tag := uint64(coordinateIndex) + 1
			if !memberOK || !coordinateOK || !indexOK || tag == 0 {
				return Selection{}, false
			}
			if _, duplicate := seen[tag]; !duplicate {
				seen[tag] = struct{}{}
				selection.tags = append(selection.tags, tag)
			}
		}
	}
	sort.Slice(selection.tags, func(left, right int) bool { return selection.tags[left] < selection.tags[right] })
	return selection, true
}

// Derive answers the return members this call site observes.
//
// A site whose evidence is beyond enumeration names no member: what such a
// site publishes is the fold's answer over an empty delivery, not a wider
// observation. The order is ascending tag, which is the order the engine
// canonicalizes a selection by, and the tags are the strictly ascending
// coordinate indices the selection named.
func Derive(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate valuedomain.MountedCallResultSlot,
	fact calldomain.Value,
) (Plan, bool) {
	selection, selectionOK := Select(values, calls, candidate, fact)
	if !selectionOK {
		return Plan{}, false
	}
	if selection.top {
		return Plan{}, true
	}
	routes := make([]Route, 0, len(selection.tags))
	for _, tag := range selection.tags {
		coordinate, coordinateOK := values.CoordinateAt(int(tag - 1))
		if !coordinateOK {
			return Plan{}, false
		}
		routes = append(routes, Route{coordinate: coordinate, tag: tag, set: true})
	}
	return Plan{routes: routes}, true
}

// callValueValid states the shape a Call fact must have before this relation
// reads it: one of the four dispositions Call's algebra publishes.
func callValueValid(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}
