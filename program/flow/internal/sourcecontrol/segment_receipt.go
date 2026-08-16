package sourcecontrol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// SegmentCarrierKind selects the independent structural witness retained by
// a route subdivision. Endpoint identity is always carried by PhaseRef; this
// carrier supplies only the existing Arc or genuine CSR NodePair relation.
type SegmentCarrierKind uint8

const (
	SegmentCarrierNone SegmentCarrierKind = iota
	SegmentCarrierArc
	SegmentCarrierNodePair
)

// SegmentCarrier is inert unless its opaque references are live under one
// Result. Callers cannot manufacture either structural coordinate.
type SegmentCarrier struct {
	kind     SegmentCarrierKind
	arc      ArcRef
	fromNode NodeRef
	toNode   NodeRef
}

func NoSegmentCarrier() SegmentCarrier { return SegmentCarrier{} }

func ArcSegmentCarrier(ref ArcRef) SegmentCarrier {
	return SegmentCarrier{kind: SegmentCarrierArc, arc: ref}
}

func NodePairSegmentCarrier(from, to NodeRef) SegmentCarrier {
	return SegmentCarrier{kind: SegmentCarrierNodePair, fromNode: from, toNode: to}
}

func (carrier SegmentCarrier) Kind() SegmentCarrierKind { return carrier.kind }
func (carrier SegmentCarrier) ArcRef() (ArcRef, bool) {
	return carrier.arc, carrier.kind == SegmentCarrierArc && carrier.arc.Available()
}
func (carrier SegmentCarrier) NodePair() (NodeRef, NodeRef, bool) {
	return carrier.fromNode, carrier.toNode,
		carrier.kind == SegmentCarrierNodePair && SameOwner(carrier.fromNode, carrier.toNode)
}

type routeSegmentLifecycle struct {
	mu   sync.Mutex
	used bool
}

type routeSegmentRelation uint8

const (
	segmentRelationInvalid routeSegmentRelation = iota
	segmentRelationRootOutcome
	segmentRelationPropagation
	segmentRelationOutcomeResume
)

// operationOutcomeRole is a closed, issuer-only refinement of a carrierless
// root→Outcome relation. A route family is not enough: this tag exists only
// after the matching typed SourceControl proof has validated its authority.
type operationOutcomeRole uint8

const (
	operationOutcomeNone operationOutcomeRole = iota
	operationOutcomeReturn
	operationOutcomeNumericFor
	operationOutcomeGenericFor
	operationOutcomeTableField
	operationOutcomeCallExceptional
	operationOutcomeCallTailReturn
)

// RouteSegmentReceipt is the exact SourceControl subdivision authority for a
// route whose endpoint includes a parent-issued Outcome phase. It is a
// one-shot capability: copies share lifecycle state, and foreign/copy
// consumption burns the exact retry.
type RouteSegmentReceipt struct {
	state      *routeSegmentLifecycle
	owner      *Result
	from       PhaseRef
	to         PhaseRef
	carrier    SegmentCarrier
	role       keyspace.ContentID
	relation   routeSegmentRelation
	operation  operationOutcomeRole
	fromTermID keyspace.ContentID
	targetID   keyspace.ContentID
	fromFamily keyspace.Family
}

type callTailReturnLifecycle struct {
	mu   sync.Mutex
	used bool
}

// CallTailReturnReceipt is an opaque parent-validated tail disposition. It
// stores only role hashes; the authored Return scan is performed once by the
// causal parent and never by SourceControl's hot route issuer.
type CallTailReturnReceipt struct {
	state   *callTailReturnLifecycle
	owner   *Result
	callID  keyspace.ContentID
	exitID  keyspace.ContentID
	ownerID keyspace.ContentID
}

func (r *Result) IssueCallTailReturnReceipt(sourceView source.View, flow authored.View, outcomes *outcome.Result, call, ret, exit, owner keyspace.Term) (*CallTailReturnReceipt, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(call) != keyspace.FamilyCall ||
		keyspace.TermFamily(ret) != keyspace.FamilyReturn || keyspace.TermFamily(exit) != keyspace.FamilyOutcome ||
		keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return nil, errors.New("program/flow/sourcecontrol: Call-tail receipt owner is unavailable")
	}
	callOwner, _, _, _, callOK := flow.Calls().Get(call)
	returnOwner, values, returnOK := flow.Control().Returns().Get(ret)
	valuesOwner, tail, valuesOK := flow.Values().Get(values)
	length, lengthOK := flow.Values().Len(values)
	rowOwner, outcomeKind, _, outcomeOK := outcomes.Get(exit)
	returnExit, exitOK := outcomes.ReturnExit(ret)
	if !callOK || !returnOK || !valuesOK || !lengthOK || callOwner != owner || returnOwner != owner ||
		valuesOwner != owner || tail != call || length != 0 || !outcomeOK || rowOwner != owner ||
		outcomeKind != kind.OutcomeReturn || !exitOK || returnExit != exit {
		return nil, errors.New("program/flow/sourcecontrol: Call-tail receipt relation is unavailable")
	}
	return &CallTailReturnReceipt{state: &callTailReturnLifecycle{}, owner: r, callID: routeTermID(call), exitID: routeTermID(exit), ownerID: routeTermID(owner)}, nil
}

func (receipt *CallTailReturnReceipt) consume(graph *Result, call, exit, owner keyspace.Term) bool {
	if receipt == nil || receipt.state == nil {
		return false
	}
	receipt.state.mu.Lock()
	defer receipt.state.mu.Unlock()
	if receipt.state.used {
		return false
	}
	receipt.state.used = true
	return receipt.owner != nil && graph == receipt.owner && graph.available() &&
		receipt.callID == routeTermID(call) && receipt.exitID == routeTermID(exit) && receipt.ownerID == routeTermID(owner)
}

// Available reports whether the subdivision authority remains live. It does
// not consume the receipt and never exposes its endpoint or carrier payload.
func (receipt *RouteSegmentReceipt) Available() bool {
	if receipt == nil || receipt.state == nil || receipt.owner == nil {
		return false
	}
	receipt.state.mu.Lock()
	defer receipt.state.mu.Unlock()
	return !receipt.state.used && receipt.owner.available() &&
		RouteSegment{from: receipt.from, to: receipt.to, carrier: receipt.carrier, role: receipt.role, relation: receipt.relation, operation: receipt.operation, fromTermID: receipt.fromTermID, targetID: receipt.targetID, fromFamily: receipt.fromFamily}.valid() && receipt.role.Available()
}

// RouteSegment is the consumed projection of a subdivision receipt. It keeps
// endpoint and structural proofs opaque while allowing recurrence/routeplan
// to pass the exact capabilities onward.
type RouteSegment struct {
	from       PhaseRef
	to         PhaseRef
	carrier    SegmentCarrier
	role       keyspace.ContentID
	relation   routeSegmentRelation
	operation  operationOutcomeRole
	fromTermID keyspace.ContentID
	targetID   keyspace.ContentID
	fromFamily keyspace.Family
}

func (segment RouteSegment) Endpoints() (PhaseRef, PhaseRef, bool) {
	return segment.from, segment.to,
		segment.from.Available() && segment.to.Available() && SamePhaseOwner(segment.from, segment.to)
}
func (segment RouteSegment) Carrier() (SegmentCarrier, bool) {
	if !segment.valid() {
		return SegmentCarrier{}, false
	}
	return segment.carrier, true
}
func (segment RouteSegment) MatchesRoute(from, to keyspace.Term) bool {
	return segment.valid() && segment.role == routeSegmentRole(from, to)
}
func (segment RouteSegment) valid() bool {
	from, to, endpointsOK := segment.Endpoints()
	if !endpointsOK || !segment.role.Available() || segment.relation == segmentRelationInvalid || !(from.OutcomePhase() || to.OutcomePhase()) {
		return false
	}
	switch segment.carrier.kind {
	case SegmentCarrierNone:
		if segment.relation == segmentRelationPropagation || segment.relation == segmentRelationOutcomeResume {
			return segment.operation == operationOutcomeNone
		}
		return segment.relation == segmentRelationRootOutcome && segment.carrierlessRootValid()
	case SegmentCarrierArc:
		if segment.relation != segmentRelationRootOutcome {
			return false
		}
		if !segment.carrier.arc.Available() || segment.carrier.arc.result != from.result {
			return false
		}
		_, arc, ok := from.result.ResolveArcRef(segment.carrier.arc)
		if !ok || !carrierSideMatches(from.result, from, arc.From) || !carrierSideMatches(from.result, to, arc.To) {
			return false
		}
		if segment.fromFamily != keyspace.FamilyBreak && segment.fromFamily != keyspace.FamilyGoto {
			return false
		}
		return routeTermID(arc.Source) == segment.fromTermID && routeTermID(arc.Target) == segment.targetID
	case SegmentCarrierNodePair:
		// An Outcome subdivision has no genuine CSR node on its Outcome side;
		// root routes use either the exact Break/Goto Arc or a carrierless
		// Return boundary. A NodePair here would be a fabricated splice.
		return false
	default:
		return false
	}
}

func (segment RouteSegment) carrierlessRootValid() bool {
	switch segment.operation {
	case operationOutcomeReturn:
		return segment.fromFamily == keyspace.FamilyReturn
	case operationOutcomeNumericFor, operationOutcomeGenericFor:
		return segment.fromFamily == keyspace.FamilyLoop
	case operationOutcomeTableField:
		return segment.fromFamily == keyspace.FamilyTableField
	case operationOutcomeCallExceptional, operationOutcomeCallTailReturn:
		return segment.fromFamily == keyspace.FamilyCall
	default:
		return false
	}
}

// phaseSplicesCSR proves an ordinary CSR endpoint is the exact phase of its
// carrier endpoint. Outcome phases never pass this predicate: their exact
// ownership is proven by the sealed segment relation at issuance.
func phaseSplicesCSR(graph *Result, phase PhaseRef, node uint32) bool {
	if graph == nil || phase.result != graph || phase.OutcomePhase() || node >= graph.NodeCount() {
		return false
	}
	path, ok := graph.VertexPathAt(node)
	return ok && path == phase.path
}

func carrierSideMatches(graph *Result, phase PhaseRef, node uint32) bool {
	return phase.OutcomePhase() || phaseSplicesCSR(graph, phase, node)
}

// IssueRouteSegmentReceipt validates both endpoint ownership and the
// independent Arc/NodePair relation while the issuing SourceControl Result
// is live. A plain endpoint pair or a foreign carrier cannot enter this
// authority.
func (r *Result) issueRouteSegmentReceipt(routeFrom, routeTo, owner keyspace.Term, from, to PhaseRef, carrier SegmentCarrier, operation operationOutcomeRole) (*RouteSegmentReceipt, error) {
	if r == nil || !r.available() || routeFrom == 0 || routeTo == 0 || !from.Available() || !to.Available() ||
		!SamePhaseOwner(from, to) || from.result != r ||
		!(from.OutcomePhase() || to.OutcomePhase()) {
		return nil, errors.New("program/flow/sourcecontrol: route segment endpoint is unavailable")
	}
	state := r.outcomePhases
	if state == nil {
		return nil, errors.New("program/flow/sourcecontrol: route segment Outcome authority is unavailable")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.owner != r || state.state != outcomePhaseIssued || !r.routePhaseMatchesLocked(state, routeFrom, from) || !r.routePhaseMatchesLocked(state, routeTo, to) {
		return nil, errors.New("program/flow/sourcecontrol: route segment endpoint does not match sealed Outcome relation")
	}
	relation, relationOK := r.routeRelationLocked(state, routeFrom, routeTo, from, to)
	if !relationOK {
		return nil, errors.New("program/flow/sourcecontrol: route segment relation is unavailable")
	}
	if relation == segmentRelationRootOutcome && !r.outcomeOwnerMatchesLocked(state, routeTo, owner) {
		return nil, errors.New("program/flow/sourcecontrol: root Outcome owner is unavailable")
	}
	segment := RouteSegment{from: from, to: to, carrier: carrier, role: routeSegmentRole(routeFrom, routeTo), relation: relation, operation: operation,
		fromTermID: routeTermID(routeFrom), fromFamily: keyspace.TermFamily(routeFrom), targetID: r.outcomeTargetIDLocked(state, routeTo)}
	if !segment.valid() {
		return nil, errors.New("program/flow/sourcecontrol: route segment carrier is unavailable")
	}
	return &RouteSegmentReceipt{state: &routeSegmentLifecycle{}, owner: r, from: from, to: to, carrier: carrier, role: segment.role, relation: relation, operation: operation,
		fromTermID: segment.fromTermID, fromFamily: segment.fromFamily, targetID: segment.targetID}, nil
}

// IssueRootOutcomeSegment issues the narrowly typed root→Outcome subdivision.
// Break/Goto roots must carry their exact structural Arc; Return roots use no
// recurrence carrier.
func (r *Result) IssueRootOutcomeSegment(sourceView source.View, outcomes *outcome.Result, routeFrom, routeTo, owner keyspace.Term, carrier SegmentCarrier) (*RouteSegmentReceipt, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) || keyspace.TermFamily(routeTo) != keyspace.FamilyOutcome || owner == 0 {
		return nil, errors.New("program/flow/sourcecontrol: root Outcome relation is unavailable")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(routeTo)
	if !rowOK || rowOwner != owner {
		return nil, errors.New("program/flow/sourcecontrol: root Outcome owner disagrees")
	}
	var exit keyspace.Term
	var exitOK bool
	switch keyspace.TermFamily(routeFrom) {
	case keyspace.FamilyReturn:
		if outcomeKind != kind.OutcomeReturn || carrier.kind != SegmentCarrierNone {
			return nil, errors.New("program/flow/sourcecontrol: Return root relation is not sealed")
		}
		exit, exitOK = outcomes.ReturnExit(routeFrom)
	case keyspace.FamilyBreak:
		if outcomeKind != kind.OutcomeBreak || carrier.kind != SegmentCarrierArc {
			return nil, errors.New("program/flow/sourcecontrol: Break root relation is not sealed")
		}
		exit, exitOK = outcomes.BreakExit(routeFrom)
	case keyspace.FamilyGoto:
		if outcomeKind != kind.OutcomeGoto || carrier.kind != SegmentCarrierArc {
			return nil, errors.New("program/flow/sourcecontrol: Goto root relation is not sealed")
		}
		exit, exitOK = outcomes.GotoExit(routeFrom)
	default:
		return nil, errors.New("program/flow/sourcecontrol: root operation kind is not sealed")
	}
	if !exitOK || exit != routeTo {
		return nil, errors.New("program/flow/sourcecontrol: root operation does not own Outcome exit")
	}
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, routeFrom, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, routeTo, false)
	if !fromOK || !toOK {
		return nil, errors.New("program/flow/sourcecontrol: root Outcome route endpoint is unavailable")
	}
	operation := operationOutcomeNone
	if keyspace.TermFamily(routeFrom) == keyspace.FamilyReturn {
		operation = operationOutcomeReturn
	}
	receipt, err := r.issueRouteSegmentReceipt(routeFrom, routeTo, owner, from, to, carrier, operation)
	if err != nil || receipt == nil || receipt.relation != segmentRelationRootOutcome {
		return nil, errors.New("program/flow/sourcecontrol: root Outcome subdivision is unavailable")
	}
	return receipt, nil
}

// IssueOutcomePropagationSegment issues only an exact child→parent Outcome
// propagation subdivision. It cannot retain an Arc or NodePair carrier.
func (r *Result) IssueOutcomePropagationSegment(sourceView source.View, outcomes *outcome.Result, fromTerm, toTerm keyspace.Term) (*RouteSegmentReceipt, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return nil, errors.New("program/flow/sourcecontrol: Outcome propagation inputs are unavailable")
	}
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, toTerm, false)
	if !fromOK || !toOK {
		return nil, errors.New("program/flow/sourcecontrol: Outcome propagation endpoint is unavailable")
	}
	receipt, err := r.issueRouteSegmentReceipt(fromTerm, toTerm, 0, from, to, NoSegmentCarrier(), operationOutcomeNone)
	if err != nil || receipt == nil || receipt.relation != segmentRelationPropagation {
		return nil, errors.New("program/flow/sourcecontrol: Outcome propagation subdivision is unavailable")
	}
	return receipt, nil
}

// IssueNumericForOutcomeSegment proves a NumericFor Loop exceptional arm.
func (r *Result) IssueNumericForOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (*RouteSegmentReceipt, error) {
	return r.issueLoopOutcomeSegment(sourceView, flow, outcomes, fromTerm, toTerm, owner, kind.LoopNumericFor, kind.OutcomeThrow)
}

// IssueGenericForOutcomeSegment proves a GenericFor Loop exceptional arm.
func (r *Result) IssueGenericForOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (*RouteSegmentReceipt, error) {
	return r.issueLoopOutcomeSegment(sourceView, flow, outcomes, fromTerm, toTerm, owner, kind.LoopGenericFor, 0)
}

func (r *Result) issueLoopOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term, loopKind kind.LoopKind, onlyKind kind.OutcomeKind) (*RouteSegmentReceipt, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyLoop {
		return nil, errors.New("program/flow/sourcecontrol: loop Outcome source is unavailable")
	}
	gotOwner, _, gotKind, _, ok := flow.Control().Loops().Get(fromTerm)
	if !ok || gotOwner != owner || gotKind != loopKind {
		return nil, errors.New("program/flow/sourcecontrol: loop Outcome role disagrees")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || (onlyKind != 0 && outcomeKind != onlyKind) || (onlyKind == 0 && outcomeKind != kind.OutcomeThrow && outcomeKind != kind.OutcomeYield && outcomeKind != kind.OutcomeCancel) {
		return nil, errors.New("program/flow/sourcecontrol: loop Outcome arm disagrees")
	}
	operation := operationOutcomeNumericFor
	if loopKind == kind.LoopGenericFor {
		operation = operationOutcomeGenericFor
	}
	return r.issueOperationRoot(sourceView, outcomes, fromTerm, toTerm, owner, operation)
}

// IssueTableFieldOutcomeSegmentWithEligibility consumes the exact Causal
// FieldKey/invalid-FieldExact proof and avoids re-importing literal
// classification into SourceControl.
func (r *Result) IssueTableFieldOutcomeSegmentWithEligibility(sourceView source.View, outcomes *outcome.Result, proof *TableFieldThrowEligibility, fromTerm, toTerm, owner keyspace.Term) (*RouteSegmentReceipt, error) {
	if !proof.consume(r, fromTerm, toTerm, owner) {
		return nil, errors.New("program/flow/sourcecontrol: TableField eligibility proof is unavailable")
	}
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return nil, errors.New("program/flow/sourcecontrol: TableField eligibility inputs are unavailable")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || outcomeKind != kind.OutcomeThrow {
		return nil, errors.New("program/flow/sourcecontrol: TableField Throw arm disagrees")
	}
	return r.issueOperationRoot(sourceView, outcomes, fromTerm, toTerm, owner, operationOutcomeTableField)
}

// IssueCallOutcomeSegment proves a CallBoundary exceptional arm.
func (r *Result) IssueCallOutcomeSegment(sourceView source.View, flow authored.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term) (*RouteSegmentReceipt, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyCall {
		return nil, errors.New("program/flow/sourcecontrol: Call Outcome source is unavailable")
	}
	callOwner, _, _, _, ok := flow.Calls().Get(fromTerm)
	if !ok || callOwner != owner {
		return nil, errors.New("program/flow/sourcecontrol: Call owner disagrees")
	}
	rowOwner, outcomeKind, _, rowOK := outcomes.Get(toTerm)
	if !rowOK || rowOwner != owner || (outcomeKind != kind.OutcomeThrow && outcomeKind != kind.OutcomeYield && outcomeKind != kind.OutcomeCancel) {
		return nil, errors.New("program/flow/sourcecontrol: Call exceptional arm disagrees")
	}
	return r.issueOperationRoot(sourceView, outcomes, fromTerm, toTerm, owner, operationOutcomeCallExceptional)
}

// IssueCallTailReturnSegmentWithReceipt consumes the one-shot parent proof and
// issues the exact Call→Return Outcome subdivision without rescanning Returns.
func (r *Result) IssueCallTailReturnSegmentWithReceipt(sourceView source.View, outcomes *outcome.Result, proof *CallTailReturnReceipt, call, toTerm, owner keyspace.Term) (*RouteSegmentReceipt, error) {
	if !proof.consume(r, call, toTerm, owner) {
		return nil, errors.New("program/flow/sourcecontrol: Call-tail parent receipt is unavailable")
	}
	if !r.matchesOutcomeInputs(sourceView, outcomes) {
		return nil, errors.New("program/flow/sourcecontrol: Call-tail parent inputs are unavailable")
	}
	rowOwner, outcomeKind, _, ok := outcomes.Get(toTerm)
	if !ok || rowOwner != owner || outcomeKind != kind.OutcomeReturn {
		return nil, errors.New("program/flow/sourcecontrol: Call-tail Return exit disagrees")
	}
	return r.issueOperationRoot(sourceView, outcomes, call, toTerm, owner, operationOutcomeCallTailReturn)
}

func (r *Result) issueOperationRoot(sourceView source.View, outcomes *outcome.Result, fromTerm, toTerm, owner keyspace.Term, operation operationOutcomeRole) (*RouteSegmentReceipt, error) {
	from, fromOK := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	to, toOK := r.ResolveRouteEndpoint(sourceView, outcomes, toTerm, false)
	if !fromOK || !toOK {
		return nil, errors.New("program/flow/sourcecontrol: operation Outcome endpoint unavailable")
	}
	return r.issueRouteSegmentReceipt(fromTerm, toTerm, owner, from, to, NoSegmentCarrier(), operation)
}

func (r *Result) matchesOutcomeInputs(sourceView source.View, outcomes *outcome.Result) bool {
	return r != nil && r.available() && sourceView.Identity().ContentID() == r.sourceID &&
		outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID)
}

func (r *Result) matchesOperationInputs(sourceView source.View, flow authored.View, outcomes *outcome.Result) bool {
	return r.matchesOutcomeInputs(sourceView, outcomes) && flow.Cold().ContentID() == r.flowID
}

// routePhaseMatches binds an endpoint term to the exact phase issued for that
// term. Non-Normal Outcome terms must use their parent-issued Outcome phase;
// Normal Outcome terms must use the owning BodyTail CSR phase. Other typed
// CSR endpoints remain checked by the structural carrier splice proof.
func (r *Result) routePhaseMatchesLocked(state *outcomePhaseLifecycle, term keyspace.Term, phase PhaseRef) bool {
	if r == nil || state == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return true
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(state.byTerm)) {
		return false
	}
	if uint64(ordinal) < uint64(len(state.nonNormal)) && state.nonNormal[ordinal] {
		return phase.OutcomePhase() && phase.result == r && phase.path == state.byTerm[ordinal]
	}
	return !phase.OutcomePhase() && uint64(ordinal) < uint64(len(state.normalByTerm)) && state.normalByTerm[ordinal].Available() && phase.path == state.normalByTerm[ordinal]
}

// routeRelationMatches proves the only phase-to-phase relation that cannot be
// recovered from CSR carrier endpoints: a child non-Normal Outcome must point
// to its exact propagated parent. Root→Outcome and Outcome→CSR-phase resume
// routes are accepted once each endpoint has passed routePhaseMatches. A
// logical Normal Outcome is a valid resume destination because it resolves to
// its owning BodyTail CSR phase.
func (r *Result) routeRelationLocked(state *outcomePhaseLifecycle, fromTerm, toTerm keyspace.Term, from, to PhaseRef) (routeSegmentRelation, bool) {
	if r == nil || state == nil {
		return segmentRelationInvalid, false
	}
	fromOutcome, fromOrdinal := r.nonNormalTermLocked(state, fromTerm)
	toOutcome, _ := r.nonNormalTermLocked(state, toTerm)
	fromFamilyOutcome := keyspace.TermFamily(fromTerm) == keyspace.FamilyOutcome
	if fromOutcome && toOutcome {
		if uint64(fromOrdinal) >= uint64(len(state.parentByTerm)) || !state.parentByTerm[fromOrdinal].Available() {
			return segmentRelationInvalid, false
		}
		return segmentRelationPropagation, from.OutcomePhase() && to.OutcomePhase() && state.parentByTerm[fromOrdinal] == to.path
	}
	if !fromFamilyOutcome && toOutcome && to.OutcomePhase() {
		return segmentRelationRootOutcome, true
	}
	if fromOutcome && from.OutcomePhase() && !to.OutcomePhase() {
		return segmentRelationOutcomeResume, true
	}
	return segmentRelationInvalid, false
}

// ResolveRouteEndpoint is SourceControl's typed endpoint resolver. It is the
// only route ingress that can turn a logical term into a live PhaseRef.
func (r *Result) ResolveRouteEndpoint(sourceView source.View, outcomes *outcome.Result, term keyspace.Term, sourcePhase bool) (PhaseRef, bool) {
	if r == nil || !r.available() || sourceView.Identity().ContentID() != r.sourceID || !outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return PhaseRef{}, false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBody:
		if sourcePhase {
			return r.BodyTailPhase(term)
		}
		return r.BodyEntryPhase(term)
	case keyspace.FamilyOutcome:
		_, outcomeKind, _, ok := outcomes.Get(term)
		if !ok {
			return PhaseRef{}, false
		}
		if outcomeKind == kind.OutcomeNormal {
			owner, _, _, ownerOK := outcomes.Get(term)
			if !ownerOK {
				return PhaseRef{}, false
			}
			return r.BodyTailPhase(owner)
		}
		return r.OutcomePhase(term)
	default:
		return r.CoordinatePhase(sourceView, term)
	}
}

func (r *Result) nonNormalTermLocked(state *outcomePhaseLifecycle, term keyspace.Term) (bool, uint32) {
	if r == nil || state == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return false, 0
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(state.nonNormal)) || !state.nonNormal[ordinal] || uint64(ordinal) >= uint64(len(state.byTerm)) || !state.byTerm[ordinal].Available() {
		return false, ordinal
	}
	return true, ordinal
}

func (r *Result) outcomeTargetIDLocked(state *outcomePhaseLifecycle, term keyspace.Term) keyspace.ContentID {
	if r == nil || state == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return keyspace.ContentID{}
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(state.targetByTerm)) {
		return keyspace.ContentID{}
	}
	return state.targetByTerm[ordinal]
}

func (r *Result) outcomeOwnerMatchesLocked(state *outcomePhaseLifecycle, term, owner keyspace.Term) bool {
	if r == nil || state == nil || owner == 0 || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) < uint64(len(state.ownerByTerm)) && state.ownerByTerm[ordinal].Available() && state.ownerByTerm[ordinal] == routeTermID(owner)
}

func samePhaseRef(left, right PhaseRef) bool {
	return left.Available() && right.Available() && left.result == right.result && left.class == right.class && left.path == right.path
}

// Consume destructively marks the receipt before checking the requested
// owner. This prevents copied or foreign graph values from probing a live
// subdivision capability.
func (receipt *RouteSegmentReceipt) Consume(owner *Result) (RouteSegment, bool) {
	if receipt == nil || receipt.state == nil {
		return RouteSegment{}, false
	}
	receipt.state.mu.Lock()
	defer receipt.state.mu.Unlock()
	if receipt.state.used {
		return RouteSegment{}, false
	}
	receipt.state.used = true
	segment := RouteSegment{from: receipt.from, to: receipt.to, carrier: receipt.carrier, role: receipt.role, relation: receipt.relation, operation: receipt.operation,
		fromTermID: receipt.fromTermID, targetID: receipt.targetID, fromFamily: receipt.fromFamily}
	if receipt.owner == nil || owner != receipt.owner || !owner.available() || !segment.valid() {
		return RouteSegment{}, false
	}
	return segment, true
}

// SegmentSubdivisionReceipt and SegmentSubdivision are descriptive aliases
// for callers that name the capability by its subdivision role.
type SegmentSubdivisionReceipt = RouteSegmentReceipt
type SegmentSubdivision = RouteSegment

func routeSegmentRole(from, to keyspace.Term) keyspace.ContentID {
	if from == 0 || to == 0 {
		return keyspace.ContentID{}
	}
	var input [8]byte
	binary.BigEndian.PutUint32(input[:4], uint32(from))
	binary.BigEndian.PutUint32(input[4:], uint32(to))
	digest := sha256.New()
	digest.Write([]byte("wippy/program/flow/route-segment-role-v1"))
	digest.Write([]byte{0})
	digest.Write(input[:])
	var result keyspace.ContentID
	copy(result[:], digest.Sum(nil))
	return result
}

// routeTermID is a sealed opaque role key for an Outcome target. It avoids
// retaining raw target Terms in SourceControl's segment authority.
func routeTermID(term keyspace.Term) keyspace.ContentID {
	if term == 0 {
		return keyspace.ContentID{}
	}
	var input [4]byte
	binary.BigEndian.PutUint32(input[:], uint32(term))
	digest := sha256.New()
	digest.Write([]byte("wippy/program/flow/route-segment-target-v1"))
	digest.Write([]byte{0})
	digest.Write(input[:])
	var result keyspace.ContentID
	copy(result[:], digest.Sum(nil))
	return result
}
