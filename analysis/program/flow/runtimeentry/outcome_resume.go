package runtimeentry

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// NormalizeOutcomeResume resolves one SourceControl continuation directly to
// this Result's executable endpoint. The returned row is immutable and carries
// both owner fences; routeplan's sealed Builder owns the only one-shot
// publication transaction.
func (r *Result) NormalizeOutcomeResume(sourceView source.View, control *sourcecontrol.Result,
	outcomes *outcome.Result, fromTerm keyspace.Term) (OutcomeResumeRow, error) {
	if r == nil || !r.available() || control == nil || r.control != control ||
		sourceView.Identity().ContentID() != r.sourceID {
		return OutcomeResumeRow{}, errors.New("program/flow/runtimeentry: Outcome resume owner is foreign")
	}
	anchor, err := control.ResolveOutcomeResume(sourceView, outcomes, fromTerm)
	if err != nil {
		return OutcomeResumeRow{}, err
	}
	fromTerm, raw, directTo, from, direct, ok := anchor.Parts(control)
	if !ok {
		return OutcomeResumeRow{}, errors.New("program/flow/runtimeentry: Outcome resume anchor is malformed")
	}
	toTerm, to := directTo, direct
	if directTo == 0 {
		var entryOK bool
		toTerm, entryOK = r.Entry(raw)
		if !entryOK {
			return OutcomeResumeRow{}, errors.New("program/flow/runtimeentry: Resume anchor has no executable Entry")
		}
		// Entry normalization can expose a Body activation anchor (notably a
		// dynamic Repeat's child Body). Resolve it through SourceControl's
		// typed route endpoint so Body phases use their exact Entry port rather
		// than the generic authored-occurrence coordinate lookup.
		to, entryOK = control.ResolveRouteEndpoint(sourceView, outcomes, toTerm, false)
		if !entryOK {
			return OutcomeResumeRow{}, errors.New("program/flow/runtimeentry: normalized Resume phase is unavailable")
		}
	}
	if fromTerm == 0 || toTerm == 0 || !from.Available() || !to.Available() || !from.OutcomePhase() || to.OutcomePhase() ||
		!sourcecontrol.SamePhaseOwner(from, to) {
		return OutcomeResumeRow{}, errors.New("program/flow/runtimeentry: normalized Outcome resume is malformed")
	}
	return OutcomeResumeRow{owner: r, control: control, fromTerm: fromTerm, toTerm: toTerm, from: from, to: to}, nil
}
