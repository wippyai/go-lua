package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// OutcomeResumeAnchor is the immutable SourceControl row for one direct
// abrupt-Outcome continuation. It stops at the sealed Label/Loop Resume
// anchor; runtime-entry normalization is performed by the runtimeentry owner.
// The row retains the exact Result fence so copied values cannot be spliced
// into another sealed graph.
type OutcomeResumeAnchor struct {
	owner    *Result
	fromTerm keyspace.Term
	anchor   keyspace.Term
	directTo keyspace.Term
	from     PhaseRef
	direct   PhaseRef
}

// ResolveOutcomeResume builds the exact direct Break/Goto Outcome continuation
// row. When Resume returns the target's owning Body, that Body is the
// structural BodyTail sentinel and is carried as directTo/direct. A different
// Body is the next executable root and remains raw for runtimeentry to
// normalize; every non-Body anchor follows the same runtimeentry path.
func (r *Result) ResolveOutcomeResume(sourceView source.View, outcomes *outcome.Result, fromTerm keyspace.Term) (OutcomeResumeAnchor, error) {
	if !r.matchesOutcomeInputs(sourceView, outcomes) || keyspace.TermFamily(fromTerm) != keyspace.FamilyOutcome {
		return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: Outcome resume relation is unavailable")
	}
	_, outcomeKind, target, rowOK := outcomes.Get(fromTerm)
	if !rowOK || (outcomeKind != kind.OutcomeBreak && outcomeKind != kind.OutcomeGoto) || target == 0 {
		return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: Outcome resume kind is not abrupt")
	}
	if _, propagated := outcomes.Propagation(fromTerm); propagated {
		return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: propagated Outcome cannot resume directly")
	}
	anchor, ok := r.Resume(target)
	if !ok {
		return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: typed Outcome target has no Resume")
	}
	from, ok := r.ResolveRouteEndpoint(sourceView, outcomes, fromTerm, true)
	if !ok || !from.OutcomePhase() {
		return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: Outcome resume source phase is unavailable")
	}
	row := OutcomeResumeAnchor{owner: r, fromTerm: fromTerm, anchor: anchor, from: from}
	if keyspace.TermFamily(anchor) == keyspace.FamilyBody {
		// Resume uses the owning Body as its end-of-body sentinel, but it also
		// returns a direct nested Body when that is the next root after a Label
		// or Loop.  Those two values are the same Term family and cannot be
		// distinguished from the Resume slice alone.  The target's exact Source
		// owner is the canonical discriminator: an anchor equal to that owner is
		// the Body-tail sentinel; a different Body is a direct root and must be
		// normalized through runtimeentry.Entry(anchor).
		targetOwner, _, _, targetPositionOK := sourceView.Index().Position(target)
		if !targetPositionOK {
			return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: Outcome resume target position is unavailable")
		}
		if targetOwner != anchor {
			return row, nil
		}
		normal, normalOK := outcomes.BodyExit(anchor, kind.OutcomeNormal)
		direct, directOK := r.ResolveRouteEndpoint(sourceView, outcomes, normal, false)
		if !normalOK || !directOK || direct.OutcomePhase() {
			return OutcomeResumeAnchor{}, errors.New("program/flow/sourcecontrol: Body Resume has no exact Normal tail")
		}
		row.directTo = normal
		row.direct = direct
	}
	return row, nil
}

func (anchor OutcomeResumeAnchor) Available(owner *Result) bool {
	return owner != nil && owner.available() && anchor.owner == owner && anchor.fromTerm != 0 && anchor.anchor != 0 &&
		anchor.from.Available() && anchor.from.result == owner && (anchor.directTo == 0) == (!anchor.direct.Available())
}

// Parts exposes the exact closed resume tuple to the neutral runtime-entry
// normalizer. directTo/direct are present together only for Body.
func (anchor OutcomeResumeAnchor) Parts(owner *Result) (fromTerm, rawAnchor, directTo keyspace.Term, from, direct PhaseRef, ok bool) {
	if !anchor.Available(owner) {
		return 0, 0, 0, PhaseRef{}, PhaseRef{}, false
	}
	return anchor.fromTerm, anchor.anchor, anchor.directTo, anchor.from, anchor.direct, true
}
