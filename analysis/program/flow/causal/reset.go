package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (s *resetState) appendEdge(from, to, owner, decision keyspace.Term, truth bool, arcIndex int) error {
	return s.appendEdgeOrigin(from, to, owner, decision, truth, arcIndex, routeplan.Origin{})
}

// appendBodyEntryEdge records Body entry -> first-root (or Normal). Branch
// and Loop children use the synthetic BodyEntry phase. A direct lexical Body
// instead supplies its exact Body->Body structural carrier, whose physical
// source is the parent-body anchor and whose target is this Body's entry.
func (s *resetState) appendBodyEntryEdge(body, to keyspace.Term, arcIndex int) error {
	if arcIndex >= 0 {
		return s.appendEdge(body, to, body, 0, false, arcIndex)
	}
	fromRef, fromOK := s.graph.BodyEntryRef(body)
	if !fromOK {
		return errors.New("program/flow/causal: Body entry phase is unavailable")
	}
	toRef, toOK := s.nodeRef(to, false)
	if !toOK {
		return errors.New("program/flow/causal: Body entry target CSR phase is unavailable")
	}
	fromPhase, fromPhaseOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, body, false)
	toPhase, toPhaseOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, to, false)
	if !fromPhaseOK || !toPhaseOK {
		return errors.New("program/flow/causal: Body entry endpoint phase is unavailable")
	}
	return s.appendEdgeOrigin(body, to, body, 0, false, -1,
		routeplan.CSRNodePair(fromPhase, toPhase, fromRef, toRef))
}

func (s *resetState) appendEdgeOrigin(from, to, owner, decision keyspace.Term, truth bool, arcIndex int, supplied routeplan.Origin) error {
	if from == 0 || to == 0 {
		return errors.New("program/flow/causal: Edge endpoint is empty")
	}
	if !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return errors.New("program/flow/causal: Edge owner Body is invalid")
	}
	if !s.validOutcomeOrLive(from) {
		return fmt.Errorf("program/flow/causal: live Edge source %v is not executable", from)
	}
	if !s.validOutcomeOrLive(to) {
		return fmt.Errorf("program/flow/causal: live Edge target %v is not executable", to)
	}
	if decision != 0 {
		if !isDecision(decision) || !s.live(decision) {
			return fmt.Errorf("program/flow/causal: live Edge decision %v is not executable", decision)
		}
	} else if truth {
		return errors.New("program/flow/causal: unguarded Edge carries truth polarity")
	}
	row := edgeRow{Edge: Edge{From: from, To: to, Decision: decision, Truth: truth}}
	if uint64(len(s.edgeRows)) >= uint64(^uint32(0)) {
		return errors.New("program/flow/causal: Edge denominator overflows")
	}
	if s.planState == nil || s.builder == nil {
		return errors.New("program/flow/causal: route plan builder is unavailable")
	}
	origin := supplied
	if !originAvailable(origin) {
		var err error
		origin, err = s.localOrigin(from, to, owner, arcIndex)
		if err != nil {
			return err
		}
	}
	ordinal := s.nextOrdinal
	if err := s.builder.Emit(routeplan.Route{From: from, To: to, Decision: decision, Truth: truth, Arm: routeplan.ArmLocal}, origin); err != nil {
		return err
	}
	s.nextOrdinal++
	if arcIndex >= 0 {
		if arcIndex >= len(s.arcOrdinal) || s.arcOrdinal[arcIndex] >= 0 {
			return errors.New("program/flow/causal: structural Arc has multiple planned routes")
		}
		s.arcOrdinal[arcIndex] = ordinal
	}
	s.edgeRows = append(s.edgeRows, row)
	s.edgeOwners = append(s.edgeOwners, owner)
	s.planOrdinals = append(s.planOrdinals, ordinal)
	return nil
}

func originAvailable(origin routeplan.Origin) bool {
	_, _, ok := origin.Endpoints()
	return ok
}

func (s *resetState) localOrigin(from, to, owner keyspace.Term, arcIndex int) (routeplan.Origin, error) {
	fromPhase, fromPhaseOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, from, true)
	toPhase, toPhaseOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, to, false)
	if !fromPhaseOK || !toPhaseOK {
		return routeplan.Origin{}, errors.New("program/flow/causal: route endpoint phase is unavailable")
	}
	if fromPhase.OutcomePhase() || toPhase.OutcomePhase() {
		if fromPhase.OutcomePhase() && !toPhase.OutcomePhase() {
			return routeplan.Origin{}, errors.New("program/flow/causal: Outcome resume must be projection-bound before row declaration")
		}
		var segment sourcecontrol.Segment
		var err error
		switch {
		case !fromPhase.OutcomePhase() && toPhase.OutcomePhase():
			switch keyspace.TermFamily(from) {
			case keyspace.FamilyLoop:
				_, _, loopKind, _, loopOK := s.flow.Control().Loops().Get(from)
				if !loopOK {
					return routeplan.Origin{}, errors.New("program/flow/causal: Loop Outcome role is unavailable")
				}
				switch loopKind {
				case kind.LoopNumericFor:
					segment, err = s.graph.NumericForOutcomeSegment(s.source, s.flow, s.outs, from, to, owner)
				case kind.LoopGenericFor:
					segment, err = s.graph.GenericForOutcomeSegment(s.source, s.flow, s.outs, from, to, owner)
				default:
					return routeplan.Origin{}, errors.New("program/flow/causal: Loop does not issue an Outcome arm")
				}
			case keyspace.FamilyTableField:
				ordinal := keyspace.TermOrdinal(from)
				if ordinal == 0 || uint64(ordinal) >= uint64(len(s.tableFieldThrowProof)) {
					return routeplan.Origin{}, errors.New("program/flow/causal: TableField eligibility proof is unavailable")
				}
				segment, err = s.graph.TableFieldOutcomeSegment(s.source, s.outs, s.tableFieldThrowProof[ordinal], from, to, owner)
				s.tableFieldThrowProof[ordinal] = sourcecontrol.TableFieldThrowEligibility{}
			case keyspace.FamilyCall:
				_, outcomeKind, _, outcomeOK := s.outs.Get(to)
				if !outcomeOK {
					return routeplan.Origin{}, errors.New("program/flow/causal: Call Outcome arm is unavailable")
				}
				if outcomeKind == kind.OutcomeReturn {
					ordinal := keyspace.TermOrdinal(from)
					if ordinal == 0 || uint64(ordinal) >= uint64(len(s.tailProofs)) || !s.tailProofs[ordinal].Available() {
						return routeplan.Origin{}, errors.New("program/flow/causal: Call-tail parent proof is unavailable")
					}
					segment, err = s.graph.CallTailOutcomeSegment(s.source, s.outs, s.tailProofs[ordinal], from, to, owner)
					s.tailProofs[ordinal] = sourcecontrol.CallTailProof{}
				} else {
					segment, err = s.graph.CallOutcomeSegment(s.source, s.flow, s.outs, from, to, owner)
				}
			default:
				carrier := sourcecontrol.NoSegmentCarrier()
				if arcIndex >= 0 {
					ref, ok := s.graph.ArcRefAt(arcIndex)
					if !ok {
						return routeplan.Origin{}, errors.New("program/flow/causal: planned structural Arc is unavailable")
					}
					carrier = sourcecontrol.ArcSegmentCarrier(ref)
				}
				segment, err = s.graph.RootOutcomeSegment(s.source, s.outs, from, to, owner, carrier)
			}
		case fromPhase.OutcomePhase() && toPhase.OutcomePhase():
			segment, err = s.graph.OutcomePropagationSegment(s.source, s.outs, from, to)
		default:
			return routeplan.Origin{}, errors.New("program/flow/causal: Outcome route relation is malformed")
		}
		if err != nil {
			return routeplan.Origin{}, err
		}
		origin, issued := routeplan.OutcomeSubdivision(s.graph, segment)
		if !issued {
			return routeplan.Origin{}, errors.New("program/flow/causal: Outcome route subdivision is unavailable")
		}
		return origin, nil
	}
	if arcIndex >= 0 {
		ref, ok := s.graph.ArcRefAt(arcIndex)
		if !ok {
			return routeplan.Origin{}, errors.New("program/flow/causal: planned structural Arc is unavailable")
		}
		arcFrom, arcTo, endpointsOK := s.graph.ResolveArcRoutePhases(s.source, s.outs, from, to, ref)
		if !endpointsOK {
			_, arc, arcOK := s.graph.ResolveArcRef(ref)
			logicalFrom, logicalFromOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, from, true)
			logicalTo, logicalToOK := s.graph.ResolveRouteEndpoint(s.source, s.outs, to, false)
			physicalFrom, physicalFromOK := s.graph.ResolveCSRPhaseNode(logicalFrom)
			physicalTo, physicalToOK := s.graph.ResolveCSRPhaseNode(logicalTo)
			return routeplan.Origin{}, fmt.Errorf("program/flow/causal: structural Arc endpoint phases disagree with logical route from-family=%d from-ordinal=%d to-family=%d to-ordinal=%d arc=%t arc-source-family=%d arc-source-ordinal=%d arc-target-family=%d arc-target-ordinal=%d logical-from=%t logical-to=%t from-match=%t to-match=%t", keyspace.TermFamily(from), keyspace.TermOrdinal(from), keyspace.TermFamily(to), keyspace.TermOrdinal(to), arcOK, keyspace.TermFamily(arc.Source), keyspace.TermOrdinal(arc.Source), keyspace.TermFamily(arc.Target), keyspace.TermOrdinal(arc.Target), logicalFromOK, logicalToOK, physicalFromOK && physicalFrom == arc.From, physicalToOK && physicalTo == arc.To)
		}
		return routeplan.CSRArcPair(arcFrom, arcTo, ref), nil
	}
	fromNode, fromNodeOK := s.nodeRef(from, true)
	toNode, toNodeOK := s.nodeRef(to, false)
	if fromNodeOK && toNodeOK {
		return routeplan.CSRNodePair(fromPhase, toPhase, fromNode, toNode), nil
	}
	return routeplan.CSRPhasePair(fromPhase, toPhase), nil
}

// nodeRef admits every endpoint that remains a real structural vertex. Every
// non-Normal Outcome is phase-only and therefore bypasses recurrence
// membership unless a separate exact carrier says otherwise.
func (s *resetState) nodeRef(term keyspace.Term, sourcePhase bool) (sourcecontrol.NodeRef, bool) {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBody:
		if sourcePhase {
			return s.graph.BodyTailRef(term)
		}
		return s.graph.BodyEntryRef(term)
	case keyspace.FamilyOutcome:
		owner, outcomeKind, _, ok := s.outs.Get(term)
		if !ok {
			return sourcecontrol.NodeRef{}, false
		}
		switch outcomeKind {
		case kind.OutcomeNormal:
			return s.graph.BodyTailRef(owner)
		default:
			return sourcecontrol.NodeRef{}, false
		}
	default:
		return s.graph.CoordinateRef(s.source, term)
	}
}

func outcomePhaseOutcome(outcomeKind kind.OutcomeKind) bool {
	return outcomeKind != kind.OutcomeNormal
}

func (s *proofState) validOutcomeOrLive(term keyspace.Term) bool {
	if keyspace.TermFamily(term) == keyspace.FamilyOutcome {
		ordinal := keyspace.TermOrdinal(term)
		return ordinal != 0 && ordinal <= s.counts[keyspace.FamilyOutcome]
	}
	return validPreTerm(term, s.counts) && s.live(term)
}

func (s *resetState) ensureMuStream(head keyspace.Term) error {
	family, ordinal := keyspace.TermFamily(head), keyspace.TermOrdinal(head)
	if (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(s.result.reset.headRanges[family])) {
		return errors.New("program/flow/causal: Mu head is outside final denominator")
	}
	if s.result.reset.headRanges[family][ordinal].start != ^uint32(0) {
		return nil
	}
	count, ok := s.recur.DecisionCount(head)
	if !ok || count < 0 {
		return errors.New("program/flow/causal: Mu decision stream is unavailable")
	}
	// A nested Mu head may be a one-decision view over an enclosing stream.
	// Reuse the existing physical stream interval when every decision already
	// has an issued position; this keeps one reset store and lets the query
	// owner distinguish head-local ranges without duplicating graph state.
	if count > 0 {
		positions := make([]uint32, count)
		allExisting := true
		for index := 0; index < count; index++ {
			decision, decisionOK := s.recur.DecisionAt(head, index)
			if !decisionOK || !isDecision(decision) {
				return errors.New("program/flow/causal: Mu decision stream contains an invalid decision")
			}
			decisionFamily, decisionOrdinal := keyspace.TermFamily(decision), keyspace.TermOrdinal(decision)
			if uint64(decisionOrdinal) >= uint64(len(s.result.reset.decisionHead[decisionFamily])) {
				return errors.New("program/flow/causal: Mu decision slot is unavailable")
			}
			if uint64(decisionOrdinal) >= uint64(len(s.result.reset.decisionRank[decisionFamily])) {
				return errors.New("program/flow/causal: Mu decision rank is unavailable")
			}
			existing := s.result.reset.decisionHead[decisionFamily][decisionOrdinal]
			if existing == 0 {
				allExisting = false
				continue
			}
			ownerStart, ownerEnd, ownerOK := s.result.muRange(existing)
			rank := s.result.reset.decisionRank[decisionFamily][decisionOrdinal]
			if !ownerOK || uint64(rank) >= uint64(ownerEnd-ownerStart) {
				return errors.New("program/flow/causal: existing Mu decision position is unavailable")
			}
			positions[index] = ownerStart + rank
		}
		if allExisting {
			for index := 1; index < len(positions); index++ {
				if positions[index] != positions[index-1]+1 {
					return errors.New("program/flow/causal: nested Mu stream is not a contiguous issued interval")
				}
			}
			s.result.reset.headRanges[family][ordinal] = range32{start: positions[0], end: positions[len(positions)-1] + 1}
			return nil
		}
	}
	start := uint32(len(s.result.reset.streams))
	for index := 0; index < count; index++ {
		decision, decisionOK := s.recur.DecisionAt(head, index)
		if !decisionOK || !isDecision(decision) {
			return errors.New("program/flow/causal: Mu decision stream contains an invalid decision")
		}
		decisionFamily, decisionOrdinal := keyspace.TermFamily(decision), keyspace.TermOrdinal(decision)
		if uint64(decisionOrdinal) >= uint64(len(s.result.reset.decisionHead[decisionFamily])) ||
			s.result.reset.decisionHead[decisionFamily][decisionOrdinal] != 0 {
			return errors.New("program/flow/causal: Mu decision appears in multiple head streams")
		}
		if uint64(len(s.result.reset.streams)) >= uint64(^uint32(0)) {
			return errors.New("program/flow/causal: Mu decision stream overflows")
		}
		s.result.reset.streams = append(s.result.reset.streams, decision)
		s.result.reset.decisionHead[decisionFamily][decisionOrdinal] = head
		s.result.reset.decisionRank[decisionFamily][decisionOrdinal] = uint32(index)
	}
	s.result.reset.headRanges[family][ordinal] = range32{start: start, end: uint32(len(s.result.reset.streams))}
	return nil
}

type arcDisposition uint8

const (
	arcUndisposed arcDisposition = iota
	arcLocal
	arcBoundaryNormal
	arcLivenessOnly
	arcDeadStatic
)

func (s *arcState) arc(sourceTerm, targetTerm, decision keyspace.Term, truth bool) (int, bool) {
	family, ordinal := keyspace.TermFamily(sourceTerm), keyspace.TermOrdinal(sourceTerm)
	if s.graph == nil || !causalArcSourceFamily(family) || ordinal == 0 || ordinal > s.counts[family] {
		return 0, false
	}
	count := s.graph.ArcCountAtSource(sourceTerm)
	if count < 0 {
		return 0, false
	}
	for cursor := 0; cursor < count; cursor++ {
		index, row, ok := s.graph.ArcAtSource(sourceTerm, cursor)
		if !ok || index < 0 || uint64(index) >= uint64(len(s.arcDisposition)) {
			return 0, false
		}
		if s.arcDisposition[index] != arcUndisposed {
			continue
		}
		if row.Source != sourceTerm {
			return 0, false
		}
		if row.Target == targetTerm && row.Decision == decision && row.Truth == truth {
			return index, true
		}
	}
	return 0, false
}

func (s *arcState) claimArc(sourceTerm, targetTerm, decision keyspace.Term, truth bool, disposition arcDisposition) (int, bool) {
	if disposition == arcUndisposed {
		return 0, false
	}
	index, ok := s.arc(sourceTerm, targetTerm, decision, truth)
	if !ok {
		return 0, false
	}
	s.arcDisposition[index] = disposition
	return index, true
}
