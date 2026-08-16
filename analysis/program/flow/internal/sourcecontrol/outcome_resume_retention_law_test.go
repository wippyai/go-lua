package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOutcomeResumeAnchorForeignConsumeClearsSharedRetention(t *testing.T) {
	retained := &Result{}
	state := &outcomeResumeAnchorState{
		owner:    retained,
		fromTerm: keyspace.MakeTerm(keyspace.FamilyOutcome, 1),
		anchor:   keyspace.MakeTerm(keyspace.FamilyBranch, 1),
		from:     PhaseRef{result: retained, class: phaseOutcome},
		direct:   PhaseRef{result: retained, class: phaseCSR},
		directTo: keyspace.MakeTerm(keyspace.FamilyOutcome, 2),
	}
	receipt := &OutcomeResumeAnchorReceipt{state: state}
	copyReceipt := *receipt
	if _, ok := ConsumeOutcomeResumeAnchor(nil, receipt); ok {
		t.Fatal("foreign owner consumed fabricated anchor")
	}
	if !state.used || state.owner != nil || state.fromTerm != 0 || state.anchor != 0 || state.directTo != 0 ||
		state.from != (PhaseRef{}) || state.direct != (PhaseRef{}) {
		t.Fatalf("terminal anchor retained proof state: %#v", state)
	}
	if _, ok := ConsumeOutcomeResumeAnchor(retained, &copyReceipt); ok {
		t.Fatal("copied terminal anchor consumed")
	}
	if state.owner != nil || state.fromTerm != 0 || state.anchor != 0 || state.directTo != 0 ||
		state.from != (PhaseRef{}) || state.direct != (PhaseRef{}) {
		t.Fatal("copied anchor probe restored cleared retention")
	}
}
