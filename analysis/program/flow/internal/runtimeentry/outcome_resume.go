package runtimeentry

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// IssueOutcomeResumeProjection consumes SourceControl's exact structural
// anchor and binds it to this owner's sole normalized runtime Entry.
func (r *Result) IssueOutcomeResumeProjection(sourceView source.View, control *sourcecontrol.Result,
	receipt *sourcecontrol.OutcomeResumeAnchorReceipt) (*OutcomeResumeProjection, error) {
	anchor, ok := sourcecontrol.ConsumeOutcomeResumeAnchor(control, receipt)
	if !ok {
		return nil, errors.New("program/flow/runtimeentry: Outcome resume anchor is unavailable")
	}
	if !r.available() || r.control != control || sourceView.Identity().ContentID() != r.sourceID || !anchor.Available(control) {
		return nil, errors.New("program/flow/runtimeentry: Outcome resume owner is foreign")
	}
	fromTerm, raw, directTo, from, direct, ok := anchor.ResumeParts(control)
	if !ok {
		return nil, errors.New("program/flow/runtimeentry: Outcome resume anchor is malformed")
	}
	toTerm, to := directTo, direct
	if directTo == 0 {
		var entryOK bool
		toTerm, entryOK = r.Entry(raw)
		if !entryOK {
			return nil, errors.New("program/flow/runtimeentry: Resume anchor has no executable Entry")
		}
		to, entryOK = control.CoordinatePhase(sourceView, toTerm)
		if !entryOK {
			return nil, errors.New("program/flow/runtimeentry: normalized Resume phase is unavailable")
		}
	}
	if fromTerm == 0 || toTerm == 0 || !from.Available() || !to.Available() || !from.OutcomePhase() || to.OutcomePhase() ||
		!sourcecontrol.SamePhaseOwner(from, to) {
		return nil, errors.New("program/flow/runtimeentry: normalized Outcome resume is malformed")
	}
	return &OutcomeResumeProjection{state: &projectionState{owner: r, control: control, fromTerm: fromTerm,
		toTerm: toTerm, from: from, to: to}}, nil
}

// ConsumeOutcomeResumeProjection consumes before owner validation. A copy or
// foreign Result cannot replay or probe the same projection.
func ConsumeOutcomeResumeProjection(owner *Result, control *sourcecontrol.Result,
	receipt *OutcomeResumeProjection) (OutcomeResumeSegment, bool) {
	if receipt == nil || receipt.state == nil {
		return OutcomeResumeSegment{}, false
	}
	state := receipt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		clearProjectionState(state)
		return OutcomeResumeSegment{}, false
	}
	segment := OutcomeResumeSegment{owner: state.owner, control: state.control, fromTerm: state.fromTerm,
		toTerm: state.toTerm, from: state.from, to: state.to}
	state.used = true
	clearProjectionState(state)
	if !segment.OwnedBy(owner, control) || segment.owner != owner || segment.control != control {
		return OutcomeResumeSegment{}, false
	}
	return segment, true
}

func clearProjectionState(state *projectionState) {
	if state == nil {
		return
	}
	state.owner = nil
	state.control = nil
	state.fromTerm = 0
	state.toTerm = 0
	state.from = sourcecontrol.PhaseRef{}
	state.to = sourcecontrol.PhaseRef{}
}

// RouteTerms is intentionally available only on the consumed closed segment;
// it cannot normalize an arbitrary term.
func (segment OutcomeResumeSegment) RouteTerms(owner *Result, control *sourcecontrol.Result) (keyspace.Term, keyspace.Term, bool) {
	if !segment.OwnedBy(owner, control) {
		return 0, 0, false
	}
	return segment.fromTerm, segment.toTerm, true
}
