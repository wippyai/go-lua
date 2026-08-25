// Package freshresult declares Value's Target fresh-result Call transfer.
//
// The rule is mounted at the call it belongs to. A fresh result is created by
// a mounted call, and what it is worth depends on which Target operation that
// call's own fact admits, so the rule is issued at occurrence/call on the stage
// where call dispatch has already published. Its publication is routed: one
// call reaches every Value coordinate its result slots name, and it carries the
// image at each of them through that row's own recency transition.
package freshresult

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Judgment is the sealed state this rule's fold is issued by. A fold whose
// answer rests on its axes' cold schemas cannot take them as parameters, so it
// names the state they are sealed into once and the family holds it.
type Judgment struct {
	values *valuedomain.Schema
	calls  *calldomain.Algebra
}

// NewJudgment seals the fold's state from the two cold schemas its answer rests
// on. They must be one Link's: a Value schema and a Call algebra of different
// Links describe different programs.
func NewJudgment(values *valuedomain.Schema, calls *calldomain.Algebra) (Judgment, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() ||
		!values.LinkOwner().Matches(calls.LinkOwner()) {
		return Judgment{}, false
	}
	return Judgment{values: values, calls: calls}, true
}

// FreshResultFact answers the fact one route publishes. The destination and the
// tag are the owner-issued halves of the member the selection observed; the
// fact is the join of the fresh values of every root this call admits at that
// destination.
//
// The observed cell is not read. A routed publication is a displacement - the
// staged value is the complete value of that coordinate after the operation -
// and a result that has just been created is not a function of what the
// predecessor left where it lands. The selection is still declared and still
// observed, because the row publishes under the support region that read
// reported.
//
// Which roots those are is decided in one place, by the same admission the
// relation derivation named its destinations with. This fold asks it rather
// than deciding again.
func (judgment Judgment) FreshResultFact(
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
	destination valuedomain.Coordinate,
	tag uint64,
	_ valuedomain.Value,
) (valuedomain.Value, structure.ReductionOutcome) {
	if judgment.values == nil || judgment.calls == nil || tag == 0 || !destination.Valid() {
		return valuedomain.Value{}, structure.Refuse
	}
	arms, armsOK := admittedArms(judgment.values, judgment.calls, candidate, callFact)
	if !armsOK || int(tag) > len(arms) {
		return valuedomain.Value{}, structure.Refuse
	}
	selected := arms[tag-1]
	if selected.coordinate != destination {
		return valuedomain.Value{}, structure.Refuse
	}
	fresh, freshOK := freshResultAt(judgment.values, selected)
	if !freshOK {
		return valuedomain.Value{}, structure.Refuse
	}
	return fresh, structure.Concrete
}
