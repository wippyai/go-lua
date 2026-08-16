package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBoundaryHooksTranslateInterleavedComponents(t *testing.T) {
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	selectOne := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	selectTwo := keyspace.MakeTerm(keyspace.FamilySelect, 2)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loopDecision := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	trace := eventTrace{events: []semanticEvent{
		{term: selectOne, component: 1},
		{term: branch, component: 0},
		{term: loopDecision, component: 1},
		{term: selectTwo, component: 0},
	}}
	work := []arcWork{
		{head: label, component: 0, first: 1, past: 4, recurrent: true},
		{head: loop, component: 1, first: 0, past: 3, recurrent: true},
	}
	result := &Result{
		annotations: make([]Annotation, len(work)),
		sourceID:    identity.ContentID{1},
		flowID:      identity.ContentID{2},
		staticID:    identity.ContentID{3},
		moduleID:    identity.ContentID{4},
		streams:     []keyspace.Term{branch, selectTwo, selectOne, loopDecision},
	}
	result.headSlots[keyspace.FamilyLabel] = make([]headSlot, 2)
	result.headSlots[keyspace.FamilyLabel][1] = headSlot{start: 0, end: 2, live: true}
	result.headSlots[keyspace.FamilyLoop] = make([]headSlot, 2)
	result.headSlots[keyspace.FamilyLoop][1] = headSlot{start: 2, end: 4, live: true}
	result.decisionSlots[keyspace.FamilyBranch] = make([]decisionSlot, 2)
	result.decisionSlots[keyspace.FamilyBranch][1] = decisionSlot{head: label, rank: 0}
	result.decisionSlots[keyspace.FamilySelect] = make([]decisionSlot, 3)
	result.decisionSlots[keyspace.FamilySelect][1] = decisionSlot{head: loop, rank: 0}
	result.decisionSlots[keyspace.FamilySelect][2] = decisionSlot{head: label, rank: 1}
	result.decisionSlots[keyspace.FamilyLoop] = make([]decisionSlot, 2)
	result.decisionSlots[keyspace.FamilyLoop][1] = decisionSlot{head: loop, rank: 1}

	if err := fillRanges(result, []keyspace.Term{label, loop}, trace, work); err != nil {
		t.Fatalf("fillRanges: %v", err)
	}
	want := []Annotation{{Head: label, First: 0, Past: 2}, {Head: loop, First: 0, Past: 2}}
	for index, expected := range want {
		got, ok := result.ArcAt(index)
		if !ok || got != expected {
			t.Fatalf("annotation %d = %#v/%v, want %#v/true", index, got, ok, expected)
		}
	}
	if count, ok := result.ResetCount(0); !ok || count != 2 {
		t.Fatalf("label reset count = %d/%v, want 2/true", count, ok)
	}
	if got, ok := result.ResetAt(1, 1); !ok || got != loopDecision {
		t.Fatalf("loop reset = %v/%v, want %v/true", got, ok, loopDecision)
	}
	if !result.ResetContains(0, selectTwo) || result.ResetContains(0, selectOne) {
		t.Fatal("interleaved component leaked into the label reset")
	}
	if !result.ResetContains(1, loopDecision) || result.ResetContains(1, branch) {
		t.Fatal("interleaved component leaked into the loop reset")
	}
}

func TestBoundaryHooksPreserveEmptyRange(t *testing.T) {
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	result := &Result{
		annotations: make([]Annotation, 1),
		sourceID:    identity.ContentID{1},
		flowID:      identity.ContentID{2},
		staticID:    identity.ContentID{3},
		moduleID:    identity.ContentID{4},
		headSlots: [keyspace.FamilyCount][]headSlot{
			keyspace.FamilyLabel: make([]headSlot, 2),
		},
	}
	result.headSlots[keyspace.FamilyLabel][1] = headSlot{live: true}
	work := []arcWork{{head: label, component: 0, recurrent: true}}
	if err := fillRanges(result, []keyspace.Term{label}, eventTrace{}, work); err != nil {
		t.Fatalf("fillRanges empty: %v", err)
	}
	annotation, ok := result.ArcAt(0)
	if !ok || annotation != (Annotation{Head: label}) {
		t.Fatalf("empty annotation = %#v/%v", annotation, ok)
	}
	if count, ok := result.ResetCount(0); !ok || count != 0 {
		t.Fatalf("empty reset count = %d/%v, want 0/true", count, ok)
	}
	if _, ok := result.ResetAt(0, 0); ok {
		t.Fatal("empty reset unexpectedly returned a decision")
	}
}

func TestBackwardGotoUsesLexicalRanksForEmptyIntervals(t *testing.T) {
	trace := eventTrace{
		labelStamp: []uint32{noStamp, 0, 0},
		gotoStamp:  []uint32{noStamp, 0, 0},
		labelRank:  []uint32{noStamp, 4, 9},
		gotoRank:   []uint32{noStamp, 8, 3},
	}
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	gotoTerm := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	if backward, ok := backwardGoto(trace, gotoTerm, label); !ok || !backward {
		t.Fatalf("backward empty goto = %v/%v, want true/true", backward, ok)
	}
	forwardLabel := keyspace.MakeTerm(keyspace.FamilyLabel, 2)
	if backward, ok := backwardGoto(trace, gotoTerm, forwardLabel); !ok || backward {
		t.Fatalf("forward empty goto = %v/%v, want false/true", backward, ok)
	}
}
