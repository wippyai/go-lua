package freshresult

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one Value coordinate this mounted call publishes a fresh result at,
// and the fresh roots admitted there.
//
// A call site whose callee is not resolved to a single Target operation is
// admitted against every operation it may reach, and each operation's outcome
// arms fill result ordinals of their own - but the Value coordinate a fresh
// result is written at is the call's own result slot, so several arms of one
// call reach ONE coordinate. The route is therefore the destination and its
// arms together: one row per coordinate, carrying every root that justifies it.
type Route struct {
	schema     *valuedomain.Schema
	coordinate valuedomain.Coordinate
	tag        uint64
	// keys are the fresh roots admitted at this coordinate, in the parent's own
	// member order. The row carries them because both halves of its publication
	// are functions of the whole set: the fact is their join, and the transition
	// it carries the destination image through is their composition.
	keys []heap.Key
	// opaque states that the call admits an alternative Call cannot name, so no
	// finite set of roots is the answer here and the coordinate takes the
	// owner's Top.
	opaque bool
}

func (route Route) valid() bool {
	return route.schema != nil && route.schema.Valid() && route.coordinate.Valid() &&
		route.tag != 0 && (route.opaque || len(route.keys) != 0)
}

// Coordinates answers the coordinate this route is observed at and the one it
// publishes at. A fresh result is written into the result slot it was issued
// for, so they are one coordinate under two declared roles.
func (route Route) Coordinates() (valuedomain.Coordinate, valuedomain.Coordinate, bool) {
	if !route.valid() {
		return valuedomain.Coordinate{}, valuedomain.Coordinate{}, false
	}
	return route.coordinate, route.coordinate, true
}

// Predicate is the owner-issued selection tag this route's cell is paired by.
// A selection reserves zero for "no member", so the tag is the one-based
// position of the route in its call's own route set.
func (route Route) Predicate() (uint64, bool) {
	if !route.valid() {
		return 0, false
	}
	return route.tag, true
}

// Age is the transition the image at this route's destination is carried
// through: every root admitted here has just been created, so a reference that
// was Recent for one of them is Recent no longer.
//
// It composes over the route's whole root set because the row publishes once
// for all of them. Composition is what makes it a function of the row rather
// than of an arm, which is what a routed carry is indexed by.
func (route Route) Age(prior valuedomain.Value) (valuedomain.Value, bool) {
	if !route.valid() {
		return valuedomain.Value{}, false
	}
	aged := prior
	for _, key := range route.keys {
		next, ok := route.schema.Age(aged, key)
		if !ok {
			return valuedomain.Value{}, false
		}
		aged = next
	}
	return aged, true
}

// Plan is one mounted call's sealed route set.
type Plan struct {
	routes []Route
}

// FreshResultRouteCount is the census of the route set.
func FreshResultRouteCount(plan Plan) int { return len(plan.routes) }

// FreshResultRouteAt answers one route in the sealed order.
func FreshResultRouteAt(plan Plan, index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	route := plan.routes[index]
	return route, route.valid()
}

// DeriveFreshResultRoutes is the sole fresh-result relation derivation: the
// Value coordinates one mounted call publishes a fresh result at.
//
// A call whose fact names no known target publishes nothing, and neither does
// one whose arms no known target reaches: both are the empty relation, which is
// the row's own authenticated empty selection rather than a fabricated Value.
// A call admitting an opaque alternative publishes the owner's Top at every
// coordinate its arms name, because no finite root set is the answer there.
//
// This signature is derived from the declaration; it is not chosen here.
func DeriveFreshResultRoutes(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
) (Plan, bool) {
	arms, armsOK := admittedArms(values, calls, candidate, callFact)
	if !armsOK {
		return Plan{}, false
	}
	routes := make([]Route, 0, len(arms))
	for index, arm := range arms {
		route := Route{schema: values, coordinate: arm.coordinate, tag: uint64(index) + 1, keys: arm.keys, opaque: arm.opaque}
		if !route.valid() {
			return Plan{}, false
		}
		routes = append(routes, route)
	}
	return Plan{routes: routes}, true
}

// arm is one destination coordinate of a call and the fresh roots admitted at
// it, in the parent's own member order.
type arm struct {
	coordinate valuedomain.Coordinate
	keys       []heap.Key
	opaque     bool
}

// admittedArms is the one place this rule decides which of a call's fresh
// results the call's own fact justifies. The relation derivation names the
// destinations it answers and the fold answers the fact at one of them, so both
// halves ask this rather than each deciding admission for itself.
func admittedArms(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
) ([]arm, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() ||
		!calls.OwnsCallCoordinate(candidate) || !values.LinkOwner().Matches(calls.LinkOwner()) {
		return nil, false
	}
	module, moduleOK := candidate.ModuleID()
	call, callOK := candidate.CallID()
	if !moduleOK || !callOK {
		return nil, false
	}
	if !validCallFact(callFact) {
		return nil, false
	}
	parent, parentOK := values.MountedCallActualsFor(module, call)
	if !parentOK {
		return nil, false
	}
	opaque := callFact.HasOpaqueAlternative() || callFact.IsTop()
	if !opaque && callFact.KnownTargetCount() == 0 {
		return nil, true
	}
	arms := make([]arm, 0, parent.FreshResultCount())
	index := make(map[valuedomain.Coordinate]int, parent.FreshResultCount())
	for ordinal := 0; ordinal < parent.FreshResultCount(); ordinal++ {
		member, memberOK := parent.FreshResultAt(ordinal)
		coordinate, coordinateOK := member.Coordinate()
		key, keyOK := member.Key()
		operation, operationOK := member.Operation()
		if !memberOK || !coordinateOK || !keyOK || !operationOK {
			return nil, false
		}
		admitted, admittedOK := reachesOperation(calls, callFact, operation)
		if !admittedOK {
			return nil, false
		}
		if !opaque && !admitted {
			continue
		}
		position, present := index[coordinate]
		if !present {
			index[coordinate] = len(arms)
			arms = append(arms, arm{coordinate: coordinate, opaque: opaque})
			position = len(arms) - 1
		}
		if !opaque {
			arms[position].keys = append(arms[position].keys, key)
		}
	}
	return arms, true
}

// reachesOperation reports whether any known target of this call selects the
// Target operation an arm was created by. Non-operation targets are
// authenticated Call alternatives that cannot select a fresh-result row.
func reachesOperation(calls *calldomain.Algebra, callFact calldomain.Value, operation vocabulary.Operation) (bool, bool) {
	for index := 0; index < callFact.KnownTargetCount(); index++ {
		target, targetOK := callFact.KnownTargetAt(index)
		if !targetOK || !calls.OwnsTarget(target) {
			return false, false
		}
		targetOperation, hasOperation := target.Operation()
		if hasOperation && targetOperation == operation {
			return true, true
		}
	}
	return false, true
}

func validCallFact(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

// freshResultAt answers the fact one route publishes: the join of the fresh
// facts of every root admitted there, or the owner's Top where the call admits
// an alternative Call cannot name.
func freshResultAt(values *valuedomain.Schema, route arm) (valuedomain.Value, bool) {
	if values == nil || !values.Valid() {
		return valuedomain.Value{}, false
	}
	if route.opaque {
		return values.Top(), true
	}
	if len(route.keys) == 0 {
		return valuedomain.Value{}, false
	}
	joined, joinedOK := values.FreshResultFact(route.keys[0], materialization.Recent)
	if !joinedOK {
		return valuedomain.Value{}, false
	}
	for _, key := range route.keys[1:] {
		fresh, freshOK := values.FreshResultFact(key, materialization.Recent)
		if !freshOK {
			return valuedomain.Value{}, false
		}
		next, nextOK := values.Join(joined, fresh)
		if !nextOK {
			return valuedomain.Value{}, false
		}
		joined = next
	}
	return joined, true
}
