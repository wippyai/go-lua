package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/keyspace"
)

func (s *resetState) appendEdge(from, to, owner, decision keyspace.Term, truth bool, arcIndex int) error {
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
	if arcIndex >= 0 {
		annotation, ok := s.recur.ArcAt(arcIndex)
		if !ok {
			return errors.New("program/flow/causal: recurrence annotation is unavailable for Arc")
		}
		if annotation.Head != 0 {
			if keyspace.TermFamily(annotation.Head) != keyspace.FamilyLabel && keyspace.TermFamily(annotation.Head) != keyspace.FamilyLoop {
				return errors.New("program/flow/causal: recurrence annotation has invalid Mu head")
			}
			count, countOK := s.recur.ResetCount(arcIndex)
			streamCount, streamOK := s.recur.DecisionCount(annotation.Head)
			if !countOK || !streamOK || count < 0 || streamCount < 0 {
				return errors.New("program/flow/causal: recurrence annotation range is unavailable")
			}
			row.Mu = annotation.Head
			if err := s.ensureMuStream(annotation.Head); err != nil {
				return err
			}
			row.resetStart, row.resetPast = annotation.First, annotation.Past
			if row.resetPast < row.resetStart || uint64(row.resetPast) > uint64(streamCount) {
				return errors.New("program/flow/causal: recurrence reset range exceeds Mu stream")
			}
		}
	}
	if from == to && row.Mu == 0 {
		return errors.New("program/flow/causal: Mu-less Edge is self-referential")
	}
	if uint64(len(s.edgeRows)) >= uint64(^uint32(0)) {
		return errors.New("program/flow/causal: Edge denominator overflows")
	}
	s.edgeRows = append(s.edgeRows, row)
	s.edgeOwners = append(s.edgeOwners, owner)
	return nil
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
