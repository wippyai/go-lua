package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/routeplan"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
		owner, outcomeKind, _, rowOK := s.outs.Get(term)
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
		case kind.OutcomeBreak, kind.OutcomeGoto:
			to, origin, err := s.outcomeResumeOrigin(term)
			if err != nil {
				return err
			}
			if err := s.appendEdgeOrigin(term, to, owner, 0, false, -1, origin); err != nil {
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

// outcomeResumeOrigin resolves the exact owner-fenced continuation before any
// RoutePlan or Edge row is declared. The immutable normalized row is the sole
// source of the emitted endpoint Term.
func (s *outcomeState) outcomeResumeOrigin(from keyspace.Term) (keyspace.Term, routeplan.Origin, error) {
	row, err := s.entries.NormalizeOutcomeResume(s.source, s.graph, s.outs, from)
	if err != nil {
		return 0, routeplan.Origin{}, err
	}
	fromRef, toRef, endpoints := row.Endpoints(s.entries, s.graph)
	if !endpoints {
		return 0, routeplan.Origin{}, errors.New("program/flow/causal: normalized Outcome resume endpoints are unavailable")
	}
	fromTerm, toTerm := row.RouteTerms()
	origin, to, valid := routeplan.OutcomeResumeSubdivision(fromRef, toRef, fromTerm, toTerm)
	if !valid || to == 0 {
		return 0, routeplan.Origin{}, errors.New("program/flow/causal: normalized Outcome resume subdivision is unavailable")
	}
	return to, origin, nil
}

func (s *outcomeState) emitOperationOutcomes() error {
	fields := s.flow.Fields()
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyTableField]; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		if !s.live(field) {
			continue
		}
		table, _, _, _, ok := fields.Get(field)
		if !ok {
			return errors.New("program/flow/causal: TableField row is unavailable")
		}
		if ordinal >= uint32(len(s.tableFieldThrowProof)) || !s.tableFieldThrowProof[ordinal].Available() {
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
