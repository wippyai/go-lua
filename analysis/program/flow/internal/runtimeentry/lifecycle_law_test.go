package runtimeentry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOutcomeResumeProjectionForeignConsumeClearsSharedRetention(t *testing.T) {
	owner := &Result{}
	control := &sourcecontrol.Result{}
	state := &projectionState{
		owner: owner, control: control,
		fromTerm: keyspace.MakeTerm(keyspace.FamilyOutcome, 1),
		toTerm:   keyspace.MakeTerm(keyspace.FamilyNil, 1),
	}
	receipt := &OutcomeResumeProjection{state: state}
	copyReceipt := *receipt
	if _, ok := ConsumeOutcomeResumeProjection(nil, nil, receipt); ok {
		t.Fatal("foreign owner consumed fabricated projection")
	}
	if !state.used || state.owner != nil || state.control != nil || state.fromTerm != 0 || state.toTerm != 0 ||
		state.from != (sourcecontrol.PhaseRef{}) || state.to != (sourcecontrol.PhaseRef{}) {
		t.Fatalf("terminal projection retained proof state: %#v", state)
	}
	if _, ok := ConsumeOutcomeResumeProjection(owner, control, &copyReceipt); ok {
		t.Fatal("copied terminal projection consumed")
	}
	if state.owner != nil || state.control != nil || state.fromTerm != 0 || state.toTerm != 0 ||
		state.from != (sourcecontrol.PhaseRef{}) || state.to != (sourcecontrol.PhaseRef{}) {
		t.Fatal("copied projection probe restored cleared retention")
	}
}

func TestEntryBuilderDeepFrameLifecycleIsIterativeAndClears(t *testing.T) {
	const depth = 1 << 16
	var entries [keyspace.FamilyCount][]keyspace.Term
	var states [keyspace.FamilyCount][]uint8
	entries[keyspace.FamilyNil] = make([]keyspace.Term, depth+1)
	states[keyspace.FamilyNil] = make([]uint8, depth+1)
	builder := entryBuilder{entries: &entries, states: &states, frames: make([]entryFrame, 0, depth)}
	for ordinal := 1; ordinal <= depth; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyNil, uint32(ordinal))
		states[keyspace.FamilyNil][ordinal] = 1
		entries[keyspace.FamilyNil][ordinal] = term
		builder.frames = append(builder.frames, entryFrame{term: term, family: keyspace.FamilyNil, ordinal: uint32(ordinal)})
	}
	builder.clearFrames()
	if len(builder.frames) != 0 {
		t.Fatal("deep failed traversal retained frames")
	}
	for ordinal := 1; ordinal <= depth; ordinal++ {
		if states[keyspace.FamilyNil][ordinal] != 0 || entries[keyspace.FamilyNil][ordinal] != 0 {
			t.Fatalf("deep cleanup left row %d covered", ordinal)
		}
	}
	endpoint := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	for ordinal := 1; ordinal <= depth; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyNil, uint32(ordinal))
		builder.frames = append(builder.frames, entryFrame{term: term, family: keyspace.FamilyNil, ordinal: uint32(ordinal)})
	}
	builder.completeFrames(endpoint)
	if len(builder.frames) != 0 {
		t.Fatal("deep completion retained frames")
	}
	for ordinal := 1; ordinal <= depth; ordinal++ {
		if states[keyspace.FamilyNil][ordinal] != 2 || entries[keyspace.FamilyNil][ordinal] != endpoint {
			t.Fatalf("deep completion missed row %d", ordinal)
		}
	}
}
