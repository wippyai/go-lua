package returnprojection

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal transfers the exact executable Return alternatives onto every targetless
// OutcomeReturn reached by the already-proved ReturnExit/Propagation chain.
// It is the only owner allowed to perform this authored traversal.
func Seal(
	sourceView source.View,
	authoredView authored.View,
	bodies *body.Result,
	outcomes *outcome.Result,
	executableResult *executable.Result,
	staticID, moduleID identity.ContentID,
) (*Result, error) {
	if sourceView.Identity().ContentID().Available() == false || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/returnprojection: owner provenance is unavailable")
	}
	sourceID, flowID := sourceView.Identity().ContentID(), authoredView.Cold().ContentID()
	if !flowID.Available() || !body.Matches(bodies, sourceID, flowID) || !outcome.Matches(outcomes, sourceID, flowID, staticID, moduleID) || !executable.Matches(executableResult, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/returnprojection: source projections disagree")
	}
	bodyCount := bodies.BodyCount()
	if bodyCount <= 0 {
		return nil, errors.New("program/flow/returnprojection: Body denominator is unavailable")
	}
	result := &Result{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID, rows: make([]row, bodyCount+1)}
	alternatives := make([][]keyspace.Term, bodyCount+1)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		returnOutcome, returnOK := outcomes.Find(bodyTerm, kind.OutcomeReturn, 0)
		if !returnOK {
			continue
		}
		result.rows[ordinal].outcome = returnOutcome
	}

	returns := authoredView.Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		returned, returnedOK := returns.At(index)
		owner, values, rowOK := returns.Get(returned)
		if !returnedOK || !rowOK {
			return nil, errors.New("program/flow/returnprojection: authored Return row is malformed")
		}
		if !executableResult.Executable(returned) {
			continue
		}
		if values == 0 || !executableResult.Executable(values) {
			return nil, errors.New("program/flow/returnprojection: executable Return Values is unavailable")
		}
		valuesOwner, _, valuesOK := authoredView.Values().Get(values)
		ownerActivation, ownerActivationOK := bodies.Activation(owner)
		valuesActivation, valuesActivationOK := bodies.Activation(valuesOwner)
		exit, exitOK := outcomes.ReturnExit(returned)
		if !valuesOK || !ownerActivationOK || !valuesActivationOK || ownerActivation != valuesActivation || !exitOK {
			return nil, errors.New("program/flow/returnprojection: Return ownership or exit is malformed")
		}
		for steps := 0; ; steps++ {
			if steps >= outcomes.Count() || exit == 0 {
				return nil, errors.New("program/flow/returnprojection: Return propagation is cyclic or unavailable")
			}
			exitBody, exitKind, exitTarget, exitOK := outcomes.Get(exit)
			exitActivation, activationOK := bodies.Activation(exitBody)
			if !exitOK || exitKind != kind.OutcomeReturn || exitTarget != 0 || !activationOK || exitActivation != ownerActivation {
				return nil, errors.New("program/flow/returnprojection: Return propagation leaves its activation")
			}
			ordinal := keyspace.TermOrdinal(exitBody)
			if ordinal == 0 || uint64(ordinal) >= uint64(len(result.rows)) || result.rows[ordinal].outcome != exit {
				return nil, errors.New("program/flow/returnprojection: Return Outcome is not the canonical Body target")
			}
			alternatives[ordinal] = append(alternatives[ordinal], values)
			next, propagated := outcomes.Propagation(exit)
			if !propagated {
				break
			}
			exit = next
		}
	}

	var offset uint32
	for ordinal := 1; ordinal < len(result.rows); ordinal++ {
		row := &result.rows[ordinal]
		if row.outcome == 0 {
			continue
		}
		row.start, row.end = offset, offset+uint32(len(alternatives[ordinal]))
		result.values = append(result.values, alternatives[ordinal]...)
		offset = row.end
	}
	if offset != uint32(len(result.values)) {
		return nil, errors.New("program/flow/returnprojection: Return alternative ranges are inconsistent")
	}
	if !result.validateResult() {
		return nil, errors.New("program/flow/returnprojection: malformed sealed projection")
	}
	result.sealed = true
	return result, nil
}
