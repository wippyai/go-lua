package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// emitOutcomes publishes only typed Outcome routes. It never treats
// Outcome.Get(target) as a generic successor: terminal Break/Goto targets
// are resolved through the typed Resume authority, while Propagation rows
// remain Outcome-to-Outcome only.
func (s *outcomeState) emitOutcomes() error {
	for index := 0; index < s.outs.Count(); index++ {
		term, ok := s.outs.At(index)
		if !ok {
			return errors.New("program/flow/causal: Outcome row is unavailable")
		}
		owner, outcomeKind, target, rowOK := s.outs.Get(term)
		if !rowOK || !validPreTerm(owner, s.counts) || keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: malformed Outcome row")
		}
		// Outcome identities are derived for every authored Body, but only an
		// executable, non-static Body can contribute a causal route.  Dead/static
		// exits remain typed proof data and deliberately emit no row here.
		if !s.live(owner) || s.static(owner) {
			continue
		}
		if next, propagated := s.outs.Propagation(term); propagated {
			if err := s.appendEdge(term, next, owner, 0, false, -1); err != nil {
				return err
			}
			continue
		}
		switch outcomeKind {
		case kind.OutcomeBreak:
			if keyspace.TermFamily(target) != keyspace.FamilyLoop {
				return errors.New("program/flow/causal: Break Outcome target is not Loop")
			}
			resume, resumeOK := s.graph.Resume(target)
			if !resumeOK {
				return errors.New("program/flow/causal: Break Loop resume is unavailable")
			}
			to, toOK := s.resumeTarget(resume)
			if !toOK {
				return errors.New("program/flow/causal: Break resume is not executable")
			}
			if err := s.appendEdge(term, to, owner, 0, false, -1); err != nil {
				return err
			}
		case kind.OutcomeGoto:
			if keyspace.TermFamily(target) != keyspace.FamilyLabel {
				return errors.New("program/flow/causal: Goto Outcome target is not Label")
			}
			resume, resumeOK := s.graph.Resume(target)
			if !resumeOK {
				return errors.New("program/flow/causal: Goto Label resume is unavailable")
			}
			to, toOK := s.resumeTarget(resume)
			if !toOK {
				return errors.New("program/flow/causal: Goto resume is not executable")
			}
			if err := s.appendEdge(term, to, owner, 0, false, -1); err != nil {
				return err
			}
		case kind.OutcomeNormal:
			// Normal Body routes are emitted from their exact lexical Body
			// context in emitStructure. The terminal activation normal exit has
			// no causal successor.
		case kind.OutcomeReturn, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel:
			// Return and application exceptional outcomes stop at their sealed
			// activation boundary once Propagation is exhausted.
		default:
			return fmt.Errorf("program/flow/causal: unsupported Outcome kind %v", outcomeKind)
		}
	}
	return s.emitOperationOutcomes()
}

func (s *outcomeState) emitOperationOutcomes() error {
	fields := s.flow.Fields()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTableField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		if !s.live(field) {
			continue
		}
		table, key, _, fieldKind, ok := fields.Get(field)
		if !ok {
			return errors.New("program/flow/causal: TableField row is unavailable")
		}
		if fieldKind != kind.FieldKey && !(fieldKind == kind.FieldExact && !s.exactFieldAvailable(key)) {
			continue
		}
		owner, ownerOK := s.bodyOf(table)
		if !ownerOK {
			return errors.New("program/flow/causal: dynamic TableField owner is unavailable")
		}
		throwExit, throwOK := s.outs.BodyExit(owner, kind.OutcomeThrow)
		if !throwOK {
			return errors.New("program/flow/causal: dynamic TableField Throw Outcome is unavailable")
		}
		if err := s.appendEdge(field, throwExit, owner, 0, false, -1); err != nil {
			return err
		}
	}
	return nil
}
