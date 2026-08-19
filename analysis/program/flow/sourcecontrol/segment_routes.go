package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// RootOutcomeSegment builds the narrowly typed root→Outcome subdivision.
// Break/Goto roots must carry their exact structural Arc; Return roots use no
// recurrence carrier.
func (r *Result) RootOutcomeSegment(sourceView source.View, outcomes *outcome.Result, routeFrom, routeTo, owner keyspace.Term, carrier SegmentCarrier) (Segment, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) || keyspace.TermFamily(routeTo) != keyspace.FamilyOutcome || owner == 0 {
		return Segment{}, errors.New("program/flow/sourcecontrol: root Outcome relation is unavailable")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(routeTo)
	if !rowOK || rowOwner != owner {
		return Segment{}, errors.New("program/flow/sourcecontrol: root Outcome owner disagrees")
	}
	var exit keyspace.Term
	var exitOK bool
	switch keyspace.TermFamily(routeFrom) {
	case keyspace.FamilyReturn:
		if outcomeKind != kind.OutcomeReturn || carrier.kind != SegmentCarrierNone {
			return Segment{}, errors.New("program/flow/sourcecontrol: Return root relation is not sealed")
		}
		exit, exitOK = outcomes.ReturnExit(routeFrom)
	case keyspace.FamilyBreak:
		if outcomeKind != kind.OutcomeBreak || carrier.kind != SegmentCarrierArc {
			return Segment{}, errors.New("program/flow/sourcecontrol: Break root relation is not sealed")
		}
		exit, exitOK = outcomes.BreakExit(routeFrom)
	case keyspace.FamilyGoto:
		if outcomeKind != kind.OutcomeGoto || carrier.kind != SegmentCarrierArc {
			return Segment{}, errors.New("program/flow/sourcecontrol: Goto root relation is not sealed")
		}
		exit, exitOK = outcomes.GotoExit(routeFrom)
	default:
		return Segment{}, errors.New("program/flow/sourcecontrol: root operation kind is not sealed")
	}
	if !exitOK || exit != routeTo {
		return Segment{}, errors.New("program/flow/sourcecontrol: root operation does not own Outcome exit")
	}
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, routeFrom, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, routeTo, false)
	if !fromOK || !toOK {
		return Segment{}, errors.New("program/flow/sourcecontrol: root Outcome route endpoint is unavailable")
	}
	operation := operationOutcomeNone
	if keyspace.TermFamily(routeFrom) == keyspace.FamilyReturn {
		operation = operationOutcomeReturn
	}
	segment, err := r.segmentForRoute(routeFrom, routeTo, owner, from, to, carrier, operation)
	if err != nil || segment.relation != segmentRelationRootOutcome {
		return Segment{}, errors.New("program/flow/sourcecontrol: root Outcome subdivision is unavailable")
	}
	return segment, nil
}

// OutcomePropagationSegment builds only an exact child→parent Outcome
// propagation subdivision. It cannot retain an Arc or NodePair carrier.
func (r *Result) OutcomePropagationSegment(sourceView source.View, outcomes *outcome.Result, fromTerm, toTerm keyspace.Term) (Segment, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return Segment{}, errors.New("program/flow/sourcecontrol: Outcome propagation inputs are unavailable")
	}
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, toTerm, false)
	if !fromOK || !toOK {
		return Segment{}, errors.New("program/flow/sourcecontrol: Outcome propagation endpoint is unavailable")
	}
	segment, err := r.segmentForRoute(fromTerm, toTerm, 0, from, to, NoSegmentCarrier(), operationOutcomeNone)
	if err != nil || segment.relation != segmentRelationPropagation {
		return Segment{}, errors.New("program/flow/sourcecontrol: Outcome propagation subdivision is unavailable")
	}
	return segment, nil
}

// NumericForOutcomeSegment proves a NumericFor Loop exceptional arm.
func (r *Result) NumericForOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (Segment, error) {
	return r.loopOutcomeSegment(sourceView, flow, outcomes, fromTerm, toTerm, owner, kind.LoopNumericFor, kind.OutcomeThrow)
}

// GenericForOutcomeSegment proves a GenericFor Loop exceptional arm.
func (r *Result) GenericForOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (Segment, error) {
	return r.loopOutcomeSegment(sourceView, flow, outcomes, fromTerm, toTerm, owner, kind.LoopGenericFor, 0)
}

func (r *Result) loopOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term, loopKind kind.LoopKind, onlyKind kind.OutcomeKind) (Segment, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyLoop {
		return Segment{}, errors.New("program/flow/sourcecontrol: loop Outcome source is unavailable")
	}
	gotOwner, _, gotKind, _, ok := flow.Control().Loops().Get(fromTerm)
	if !ok || gotOwner != owner || gotKind != loopKind {
		return Segment{}, errors.New("program/flow/sourcecontrol: loop Outcome role disagrees")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || (onlyKind != 0 && outcomeKind != onlyKind) || (onlyKind == 0 && outcomeKind != kind.OutcomeThrow && outcomeKind != kind.OutcomeYield && outcomeKind != kind.OutcomeCancel) {
		return Segment{}, errors.New("program/flow/sourcecontrol: loop Outcome arm disagrees")
	}
	operation := operationOutcomeNumericFor
	if loopKind == kind.LoopGenericFor {
		operation = operationOutcomeGenericFor
	}
	return r.operationRootSegment(sourceView, outcomes, fromTerm, toTerm, owner, operation)
}

// TableFieldOutcomeSegment accepts the exact Causal FieldKey/invalid-
// FieldExact row and avoids re-importing literal classification here.
func (r *Result) TableFieldOutcomeSegment(sourceView source.View, outcomes *outcome.Result, eligibility TableFieldThrowEligibility, fromTerm, toTerm, owner keyspace.Term) (Segment, error) {
	if !eligibility.ValidFor(r, fromTerm, toTerm, owner) {
		return Segment{}, errors.New("program/flow/sourcecontrol: TableField eligibility is unavailable")
	}
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return Segment{}, errors.New("program/flow/sourcecontrol: TableField eligibility inputs are unavailable")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || outcomeKind != kind.OutcomeThrow {
		return Segment{}, errors.New("program/flow/sourcecontrol: TableField Throw arm disagrees")
	}
	return r.operationRootSegment(sourceView, outcomes, fromTerm, toTerm, owner, operationOutcomeTableField)
}

// CallOutcomeSegment proves a CallBoundary exceptional arm.
func (r *Result) CallOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (Segment, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyCall {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call Outcome source is unavailable")
	}
	callOwner, _, _, _, ok := flow.Calls().Get(fromTerm)
	if !ok || callOwner != owner {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call owner disagrees")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || (outcomeKind != kind.OutcomeThrow && outcomeKind != kind.OutcomeYield && outcomeKind != kind.OutcomeCancel) {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call exceptional arm disagrees")
	}
	return r.operationRootSegment(sourceView, outcomes, fromTerm, toTerm, owner, operationOutcomeCallExceptional)
}

// CallTailOutcomeSegment accepts the immutable parent row and builds the exact
// Call→Return Outcome subdivision without rescanning Returns.
func (r *Result) CallTailOutcomeSegment(sourceView source.View, outcomes *outcome.Result, proof CallTailProof, call, toTerm, owner keyspace.Term) (Segment, error) {
	if !proof.ValidFor(r, call, toTerm, owner) {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call-tail parent row is unavailable")
	}
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call-tail parent inputs are unavailable")
	}
	rowOwner, outcomeKind, _, ok := outcomes.Get(toTerm)
	if !ok || rowOwner != owner || outcomeKind != kind.OutcomeReturn {
		return Segment{}, errors.New("program/flow/sourcecontrol: Call-tail Return exit disagrees")
	}
	return r.operationRootSegment(sourceView, outcomes, call, toTerm, owner, operationOutcomeCallTailReturn)
}

func (r *Result) operationRootSegment(sourceView source.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term, operation operationOutcomeRole) (Segment, error) {
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, toTerm, false)
	if !fromOK || !toOK {
		return Segment{}, errors.New("program/flow/sourcecontrol: operation Outcome endpoint unavailable")
	}
	return r.segmentForRoute(fromTerm, toTerm, owner, from, to, NoSegmentCarrier(), operation)
}

func (r *Result) matchesOutcomeInputs(sourceView source.View, outcomes *outcome.Result) bool {
	return r != nil && r.available() && sourceView.Identity().ContentID() == r.sourceID &&
		outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID)
}

func (r *Result) matchesOperationInputs(sourceView source.View, flow authored.View, outcomes *outcome.Result) bool {
	return r.matchesOutcomeInputs(sourceView, outcomes) && flow.Cold().ContentID() == r.flowID
}
