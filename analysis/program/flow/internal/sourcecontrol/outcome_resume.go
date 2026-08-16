package sourcecontrol

import (
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// OutcomeResumeAnchorReceipt is SourceControl's one-shot proof of the
// structural half of a direct abrupt-Outcome resume. It deliberately stops at
// the sealed Label/Loop Resume anchor: runtime Entry normalization belongs to
// the later runtimeentry owner.
type OutcomeResumeAnchorReceipt struct {
	state *outcomeResumeAnchorState
}

type outcomeResumeAnchorState struct {
	mu       sync.Mutex
	used     bool
	owner    *Result
	fromTerm keyspace.Term
	anchor   keyspace.Term
	directTo keyspace.Term
	from     PhaseRef
	direct   PhaseRef
}

// OutcomeResumeAnchor is the consumed structural proof. Terms are exposed
// only through this closed typed value; it is not a generic endpoint mapper.
type OutcomeResumeAnchor struct {
	owner    *Result
	fromTerm keyspace.Term
	anchor   keyspace.Term
	directTo keyspace.Term
	from     PhaseRef
	direct   PhaseRef
}

// IssueOutcomeResumeAnchor proves the exact direct Break/Goto Outcome and its
// sealed Label/Loop Resume. A Body resume is already a structural BodyTail
// phase and is carried as directTo/direct; every other anchor remains
// unresolved until runtimeentry consumes the receipt.
func (r *Result) IssueOutcomeResumeAnchor(sourceView source.View, outcomes *outcome.Result, fromTerm keyspace.Term) (*OutcomeResumeAnchorReceipt, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyOutcome {
		return nil, errors.New("program/flow/sourcecontrol: Outcome resume relation is unavailable")
	}
	_, outcomeKind, target, rowOK := outcomes.Get(fromTerm)
	if !rowOK || (outcomeKind != kind.OutcomeBreak && outcomeKind != kind.OutcomeGoto) || target == 0 {
		return nil, errors.New("program/flow/sourcecontrol: Outcome resume kind is not abrupt")
	}
	if _, propagated := outcomes.Propagation(fromTerm); propagated {
		return nil, errors.New("program/flow/sourcecontrol: propagated Outcome cannot resume directly")
	}
	anchor, ok := r.Resume(target)
	if !ok {
		return nil, errors.New("program/flow/sourcecontrol: typed Outcome target has no Resume")
	}
	from, ok := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	if !ok || !from.OutcomePhase() {
		return nil, errors.New("program/flow/sourcecontrol: Outcome resume source phase is unavailable")
	}
	state := &outcomeResumeAnchorState{owner: r, fromTerm: fromTerm, anchor: anchor, from: from}
	if keyspace.TermFamily(anchor) == keyspace.FamilyBody {
		normal, normalOK := outcomes.BodyExit(anchor, kind.OutcomeNormal)
		direct, directOK := r.ResolveRouteEndpoint(sourceView, outcomes, normal, false)
		if !normalOK || !directOK || direct.OutcomePhase() {
			return nil, errors.New("program/flow/sourcecontrol: Body Resume has no exact Normal tail")
		}
		state.directTo = normal
		state.direct = direct
	}
	return &OutcomeResumeAnchorReceipt{state: state}, nil
}

// ConsumeOutcomeResumeAnchor consumes before validating the requested owner,
// so copied receipts and foreign probes are terminal and fail closed.
func ConsumeOutcomeResumeAnchor(owner *Result, receipt *OutcomeResumeAnchorReceipt) (OutcomeResumeAnchor, bool) {
	if receipt == nil || receipt.state == nil {
		return OutcomeResumeAnchor{}, false
	}
	state := receipt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		clearOutcomeResumeAnchorState(state)
		return OutcomeResumeAnchor{}, false
	}
	anchor := OutcomeResumeAnchor{owner: state.owner, fromTerm: state.fromTerm, anchor: state.anchor,
		directTo: state.directTo, from: state.from, direct: state.direct}
	state.used = true
	clearOutcomeResumeAnchorState(state)
	if owner == nil || !owner.available() || anchor.owner != owner || !anchor.from.Available() || anchor.from.result != owner ||
		anchor.fromTerm == 0 || anchor.anchor == 0 || (anchor.directTo == 0) != (!anchor.direct.Available()) {
		return OutcomeResumeAnchor{}, false
	}
	return anchor, true
}

func clearOutcomeResumeAnchorState(state *outcomeResumeAnchorState) {
	if state == nil {
		return
	}
	state.owner = nil
	state.fromTerm = 0
	state.anchor = 0
	state.directTo = 0
	state.from = PhaseRef{}
	state.direct = PhaseRef{}
}

func (anchor OutcomeResumeAnchor) Available(owner *Result) bool {
	return owner != nil && owner.available() && anchor.owner == owner && anchor.fromTerm != 0 && anchor.anchor != 0 &&
		anchor.from.Available() && anchor.from.result == owner && (anchor.directTo == 0) == (!anchor.direct.Available())
}

// ResumeParts exposes the exact closed resume tuple to the neutral
// runtimeentry binder. directTo/direct are present together only for Body.
func (anchor OutcomeResumeAnchor) ResumeParts(owner *Result) (fromTerm, rawAnchor, directTo keyspace.Term, from, direct PhaseRef, ok bool) {
	if !anchor.Available(owner) {
		return 0, 0, 0, PhaseRef{}, PhaseRef{}, false
	}
	return anchor.fromTerm, anchor.anchor, anchor.directTo, anchor.from, anchor.direct, true
}
