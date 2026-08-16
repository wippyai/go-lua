package continuation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal constructs the canonical Cell and Guard continuation projections from
// one committed Source, authored Flow, and the complete owner proofs.  The
// authored View is consulted only for the existing Function, Bind, Loop, and
// Cell relations needed to validate lexical groups; it is not retained.
func Seal(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	bindingResult binding.Result,
	executableResult *executable.Result,
	candidateResult *candidates.Result,
	causalResult *causal.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	input, err := validateInputs(sourceView, flow, staticID, moduleID, bodies, bindingResult, executableResult, candidateResult, causalResult)
	if err != nil {
		return nil, err
	}
	cells, err := newCellSeal(input)
	if err != nil {
		return nil, err
	}
	guards, err := newGuardSeal(input)
	if err != nil {
		return nil, err
	}
	result := &Result{
		sourceID: sourceView.Identity().ContentID(),
		flowID:   flow.Cold().ContentID(),
		staticID: staticID,
		moduleID: moduleID,
		cells: cellProjection{
			roots:  cells.roots,
			nodes:  cells.store.nodes,
			terms:  cells.store.terms,
			counts: input.counts,
		},
		guards: guardProjection{
			roots:    guards.roots,
			nodes:    guards.nodes,
			counts:   input.counts,
			families: guards.families,
		},
	}
	if err := validatePublishedProjection(result, input.counts); err != nil {
		return nil, err
	}
	if !result.available() {
		return nil, errors.New("program/flow/continuation: result provenance is unavailable")
	}
	return result, nil
}
