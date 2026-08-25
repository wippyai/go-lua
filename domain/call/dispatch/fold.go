package dispatch

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/dispatch/route"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed semantic state of Call dispatch: the three cold
// authorities its answer rests on. Call owns the coordinate the answer
// publishes at and the alternatives it may name, Value owns the callee image
// the answer is derived from, and Heap owns the objects a rooted callee
// resolves through.
//
// It is the family's state, not a rule payload. All three are immutable for
// the life of the binding that issued them, so the state is sealed once when
// the family is installed and every invocation reads it. None of them is ever
// a parameter of the fold: the fold takes the mounted call it is indexed by
// and the one Value fact it read, and nothing else.
type Judgment struct {
	calls  *calldomain.Algebra
	values *valuedomain.Schema
	heaps  heapdomain.Schema
}

// Derive seals the state from the three authorities the declaration names.
// They must belong to one Link: a mounted call joins a Call row to the Value
// image of its callee, and owners of different Links have no such row.
func Derive(calls *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema) (Judgment, bool) {
	judgment := Judgment{calls: calls, values: values, heaps: heaps}
	if !judgment.Valid() {
		return Judgment{}, false
	}
	return judgment, true
}

// Valid reports whether this state was sealed by Derive over one Link's
// authorities.
func (judgment Judgment) Valid() bool {
	return judgment.calls != nil && judgment.calls.Valid() &&
		judgment.values != nil && judgment.values.Valid() && judgment.heaps.Valid() &&
		judgment.values.OwnsHeapSchema(judgment.heaps) &&
		judgment.values.LinkOwner().Matches(judgment.calls.LinkOwner()) &&
		judgment.heaps.LinkOwner().Matches(judgment.calls.LinkOwner())
}

// Dispatch is the one irreducible judgment of Call dispatch: the Call fact one
// mounted application publishes, given the Value image of the callee it
// applies.
//
// The alternatives are the relation Call derives for this candidate - an exact
// target, the authenticated opaque callable, or the top disposition - and the
// answer is their join, because a call site that may reach several callees
// reaches all of them. Each alternative is decoded through the candidate that
// issued it, so a predicate this application did not issue names nothing.
//
// A callee that reduces to no callable alternative at all is an explicitly
// empty selection rather than a refusal: the relation was derived and named
// nothing, which is a different answer from declining to look.
func (judgment Judgment) Dispatch(candidate calldomain.CallCoordinate, callee valuedomain.Value) (calldomain.Value, structure.ReductionOutcome) {
	if !judgment.Valid() {
		return calldomain.Value{}, structure.Refuse
	}
	plan, planOK := route.Derive(judgment.calls, judgment.values, judgment.heaps, candidate, callee)
	if !planOK {
		return calldomain.Value{}, structure.Refuse
	}
	count := route.Count(plan)
	if count < 0 {
		return calldomain.Value{}, structure.Refuse
	}
	if count == 0 {
		return calldomain.Value{}, structure.NoSelection
	}
	answer := judgment.calls.Bottom()
	for index := 0; index < count; index++ {
		alternative, alternativeOK := route.At(plan, index)
		if !alternativeOK {
			return calldomain.Value{}, structure.Refuse
		}
		predicate, predicateOK := alternative.Predicate()
		if !predicateOK {
			return calldomain.Value{}, structure.Refuse
		}
		fact, factOK := candidate.DispatchValueForPredicate(predicate)
		if !factOK {
			return calldomain.Value{}, structure.Refuse
		}
		joined, joinOK := judgment.calls.Join(answer, fact)
		if !joinOK {
			return calldomain.Value{}, structure.Refuse
		}
		answer = joined
	}
	return answer, structure.Concrete
}
