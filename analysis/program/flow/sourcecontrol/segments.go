package sourcecontrol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
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

func (carrier SegmentCarrier) Kind() SegmentCarrierKind { return carrier.kind }
func (carrier SegmentCarrier) ArcRef() (ArcRef, bool) {
	return carrier.arc, carrier.kind == SegmentCarrierArc && carrier.arc.Available()
}
func (carrier SegmentCarrier) NodePair() (NodeRef, NodeRef, bool) {
	return carrier.fromNode, carrier.toNode,
		carrier.kind == SegmentCarrierNodePair && SameOwner(carrier.fromNode, carrier.toNode)
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

// Segment is the immutable SourceControl route subdivision row. It is
// owner-fenced: the Result that sealed it is retained privately and every
// consumer must validate against that exact owner before using the row.
type Segment struct {
	owner      *Result
	from       PhaseRef
	to         PhaseRef
	carrier    SegmentCarrier
	role       identity.ContentID
	relation   routeSegmentRelation
	operation  operationOutcomeRole
	fromTermID identity.ContentID
	targetID   identity.ContentID
	fromFamily keyspace.Family
}

// CallTailProof is the immutable causal row for a call that forwards an empty
// result list into its caller's Return boundary. It retains only the owner
// fence and sealed identities; causal owns the one-shot builder transaction
// that consumes the row.
type CallTailProof struct {
	owner   *Result
	callID  identity.ContentID
	exitID  identity.ContentID
	ownerID identity.ContentID
}

func (proof CallTailProof) Available() bool {
	return proof.owner != nil && proof.owner.available() && proof.callID.Available() && proof.exitID.Available() && proof.ownerID.Available()
}

func (r *Result) CallTailProof(sourceView source.View, flow authored.View, outcomes *outcome.Result, call, ret, exit, owner keyspace.Term) (CallTailProof, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(call) != keyspace.FamilyCall ||
		keyspace.TermFamily(ret) != keyspace.FamilyReturn || keyspace.TermFamily(exit) != keyspace.FamilyOutcome ||
		keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return CallTailProof{}, errors.New("program/flow/sourcecontrol: Call-tail owner is unavailable")
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
		return CallTailProof{}, errors.New("program/flow/sourcecontrol: Call-tail relation is unavailable")
	}
	return CallTailProof{owner: r, callID: routeTermID(call), exitID: routeTermID(exit), ownerID: routeTermID(owner)}, nil
}

// ValidFor checks the exact source-control owner and authored route identities.
// It is deliberately non-consuming: the causal seal transaction clears its
// own dense proof slot after publishing the corresponding route row.
func (proof CallTailProof) ValidFor(graph *Result, call, exit, owner keyspace.Term) bool {
	return proof.owner != nil && graph == proof.owner && graph.available() &&
		proof.callID == routeTermID(call) && proof.exitID == routeTermID(exit) && proof.ownerID == routeTermID(owner)
}

func (segment Segment) Endpoints() (PhaseRef, PhaseRef, bool) {
	return segment.from, segment.to,
		segment.from.Available() && segment.to.Available() && SamePhaseOwner(segment.from, segment.to)
}
func (segment Segment) Carrier() (SegmentCarrier, bool) {
	if !segment.valid() {
		return SegmentCarrier{}, false
	}
	return segment.carrier, true
}
func (segment Segment) MatchesRoute(from, to keyspace.Term) bool {
	return segment.valid() && segment.role == routeSegmentRole(from, to)
}
func (segment Segment) valid() bool {
	if segment.owner == nil || !segment.owner.available() {
		return false
	}
	from, to, endpointsOK := segment.Endpoints()
	if !endpointsOK || from.result != segment.owner || to.result != segment.owner || !segment.role.Available() || segment.relation == segmentRelationInvalid || (!from.OutcomePhase() && !to.OutcomePhase()) {
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

func (segment Segment) carrierlessRootValid() bool {
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

// segmentForRoute validates both endpoint ownership and the independent
// Arc/NodePair relation while the SourceControl Result is live. A plain
// endpoint pair or a foreign carrier cannot enter this immutable row.
func (r *Result) segmentForRoute(routeFrom, routeTo, owner keyspace.Term, from, to PhaseRef, carrier SegmentCarrier, operation operationOutcomeRole) (Segment, error) {
	if r == nil || !r.available() || routeFrom == 0 || routeTo == 0 || !from.Available() || !to.Available() ||
		!SamePhaseOwner(from, to) || from.result != r ||
		!from.OutcomePhase() && !to.OutcomePhase() {
		return Segment{}, errors.New("program/flow/sourcecontrol: route segment endpoint is unavailable")
	}
	view := r.outcomePhases
	if view == nil {
		return Segment{}, errors.New("program/flow/sourcecontrol: route segment Outcome authority is unavailable")
	}
	if !view.Matches(r) || !r.routePhaseMatches(view, routeFrom, from) || !r.routePhaseMatches(view, routeTo, to) {
		return Segment{}, errors.New("program/flow/sourcecontrol: route segment endpoint does not match sealed Outcome relation")
	}
	relation, relationOK := r.routeRelation(view, routeFrom, routeTo, from, to)
	if !relationOK {
		return Segment{}, errors.New("program/flow/sourcecontrol: route segment relation is unavailable")
	}
	if relation == segmentRelationRootOutcome && !r.outcomeOwnerMatches(view, routeTo, owner) {
		return Segment{}, errors.New("program/flow/sourcecontrol: root Outcome owner is unavailable")
	}
	segment := Segment{owner: r, from: from, to: to, carrier: carrier, role: routeSegmentRole(routeFrom, routeTo), relation: relation, operation: operation,
		fromTermID: routeTermID(routeFrom), fromFamily: keyspace.TermFamily(routeFrom), targetID: r.outcomeTargetID(view, routeTo)}
	if !segment.valid() {
		return Segment{}, errors.New("program/flow/sourcecontrol: route segment carrier is unavailable")
	}
	return segment, nil
}

// routePhaseMatches binds an endpoint term to the exact phase built for that
// term. Non-Normal Outcome terms must use their parent-issued Outcome phase;
// Normal Outcome terms must use the owning BodyTail CSR phase. Other typed
// CSR endpoints remain checked by the structural carrier splice proof.
func (r *Result) routePhaseMatches(view *OutcomePhases, term keyspace.Term, phase PhaseRef) bool {
	if r == nil || view == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return true
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.byTerm)) {
		return false
	}
	if uint64(ordinal) < uint64(len(view.nonNormal)) && view.nonNormal[ordinal] {
		return phase.OutcomePhase() && phase.result == r && phase.path == view.byTerm[ordinal]
	}
	return !phase.OutcomePhase() && uint64(ordinal) < uint64(len(view.normalByTerm)) && view.normalByTerm[ordinal].Available() && phase.path == view.normalByTerm[ordinal]
}

// routeRelationMatches proves the only phase-to-phase relation that cannot be
// recovered from CSR carrier endpoints: a child non-Normal Outcome must point
// to its exact propagated parent. Root→Outcome and Outcome→CSR-phase resume
// routes are accepted once each endpoint has passed routePhaseMatches. A
// logical Normal Outcome is a valid resume destination because it resolves to
// its owning BodyTail CSR phase.
func (r *Result) routeRelation(view *OutcomePhases, fromTerm, toTerm keyspace.Term, from, to PhaseRef) (routeSegmentRelation, bool) {
	if r == nil || view == nil {
		return segmentRelationInvalid, false
	}
	fromOutcome, fromOrdinal := r.nonNormalTerm(view, fromTerm)
	toOutcome, _ := r.nonNormalTerm(view, toTerm)
	fromFamilyOutcome := keyspace.TermFamily(fromTerm) == keyspace.FamilyOutcome
	if fromOutcome && toOutcome {
		if uint64(fromOrdinal) >= uint64(len(view.parentByTerm)) || !view.parentByTerm[fromOrdinal].Available() {
			return segmentRelationInvalid, false
		}
		return segmentRelationPropagation, from.OutcomePhase() && to.OutcomePhase() && view.parentByTerm[fromOrdinal] == to.path
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
		if sourcePhase {
			return r.CoordinatePhase(sourceView, term)
		}
		return r.EntryPhase(sourceView, term)
	}
}

func (r *Result) nonNormalTerm(view *OutcomePhases, term keyspace.Term) (bool, uint32) {
	if r == nil || view == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return false, 0
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.nonNormal)) || !view.nonNormal[ordinal] || uint64(ordinal) >= uint64(len(view.byTerm)) || !view.byTerm[ordinal].Available() {
		return false, ordinal
	}
	return true, ordinal
}

func (r *Result) outcomeTargetID(view *OutcomePhases, term keyspace.Term) identity.ContentID {
	if r == nil || view == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return identity.ContentID{}
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.targetByTerm)) {
		return identity.ContentID{}
	}
	return view.targetByTerm[ordinal]
}

func (r *Result) outcomeOwnerMatches(view *OutcomePhases, term, owner keyspace.Term) bool {
	if r == nil || view == nil || owner == 0 || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) < uint64(len(view.ownerByTerm)) && view.ownerByTerm[ordinal].Available() && view.ownerByTerm[ordinal] == routeTermID(owner)
}

// Valid checks the immutable row against the exact SourceControl owner. The
// route-plan builder owns nonreplay by admitting each row once in its sealed
// preparation transaction; this row itself has no mutable lifecycle.
func (segment Segment) Valid(owner *Result) bool {
	return owner != nil && segment.owner == owner && segment.valid()
}

func routeSegmentRole(from, to keyspace.Term) identity.ContentID {
	if from == 0 || to == 0 {
		return identity.ContentID{}
	}
	var input [8]byte
	binary.BigEndian.PutUint32(input[:4], uint32(from))
	binary.BigEndian.PutUint32(input[4:], uint32(to))
	digest := sha256.New()
	digest.Write([]byte("wippy/program/flow/route-segment-role-v1"))
	digest.Write([]byte{0})
	digest.Write(input[:])
	var result identity.ContentID
	copy(result[:], digest.Sum(nil))
	return result
}

// routeTermID is a sealed opaque role key for an Outcome target. It avoids
// retaining raw target Terms in SourceControl's segment authority.
func routeTermID(term keyspace.Term) identity.ContentID {
	if term == 0 {
		return identity.ContentID{}
	}
	var input [4]byte
	binary.BigEndian.PutUint32(input[:], uint32(term))
	digest := sha256.New()
	digest.Write([]byte("wippy/program/flow/route-segment-target-v1"))
	digest.Write([]byte{0})
	digest.Write(input[:])
	var result identity.ContentID
	copy(result[:], digest.Sum(nil))
	return result
}
