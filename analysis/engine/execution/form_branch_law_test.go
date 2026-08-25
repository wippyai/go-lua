package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// form_branch_law_test.go states the A form's cadence, and in particular the
// one place it deliberately differs from the routed cadence beside it.

// branchLawReducer answers a scripted disposition per branch ordinal.
type branchLawReducer struct {
	verdicts map[uint64]structure.ReductionOutcome
	empty    structure.ReductionOutcome
	seen     []uint64
}

func (reducer *branchLawReducer) Reduce(branch uint64) structure.ReductionOutcome {
	reducer.seen = append(reducer.seen, branch)
	if outcome, scripted := reducer.verdicts[branch]; scripted {
		return outcome
	}
	return structure.NoSelection
}

func (reducer *branchLawReducer) Empty() structure.ReductionOutcome { return reducer.empty }

// TestOnlyTheBranchesTheTriggerNamesActivate is the cadence itself, and the
// difference from FoldSelectedRoute: an unnamed branch is the ordinary case,
// not a refusal, so the branches around it still settle.
func TestOnlyTheBranchesTheTriggerNamesActivate(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	reducer := &branchLawReducer{verdicts: map[uint64]structure.ReductionOutcome{
		0: structure.Concrete,
		1: structure.NoSelection,
		2: structure.Concrete,
	}}
	outcome := FoldBranchSet(run, &ticket, 3, reducer)
	if outcome != structure.Concrete {
		t.Fatalf("a trigger that named two of three branches settled %v", outcome)
	}
	if len(reducer.seen) != 3 {
		t.Fatalf("the fold visited %d branches, want every one of them", len(reducer.seen))
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit")
	}
	_, _, branches, drained := run.Consume()
	if !drained || len(branches) != 2 || branches[0] != 0 || branches[1] != 2 {
		t.Fatalf("published branches = %v, want exactly the two the trigger named", branches)
	}
}

// TestATriggerThatNamesNoBranchStillConcludes is the distinction the hand lane
// spells as an empty locator batch. It is not a refusal and not an absence of
// an answer: the trigger was evaluated and instantiates nothing.
func TestATriggerThatNamesNoBranchStillConcludes(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	reducer := &branchLawReducer{verdicts: map[uint64]structure.ReductionOutcome{}}
	if outcome := FoldBranchSet(run, &ticket, 2, reducer); outcome != structure.Concrete {
		t.Fatalf("a trigger that named no branch settled %v, want the concluded row", outcome)
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("submit")
	}
	if _, _, branches, drained := run.Consume(); !drained || len(branches) != 0 {
		t.Fatalf("a trigger that named nothing published %d branches", len(branches))
	}
}

// TestABranchThatRefusesEndsTheRow keeps the other four dispositions
// propagating. NoSelection is the ONE verdict that means "not this branch";
// every other declared disposition is about the row.
func TestABranchThatRefusesEndsTheRow(t *testing.T) {
	for _, verdict := range []structure.ReductionOutcome{structure.Refuse, structure.NoCandidate, structure.AuthenticatedOpaque} {
		fixture := newExecutionFixture(t)
		run := NewRun(0, 0)
		ticket := issueExecution(t, run, fixture)
		reducer := &branchLawReducer{verdicts: map[uint64]structure.ReductionOutcome{
			0: structure.Concrete,
			1: verdict,
		}}
		if outcome := FoldBranchSet(run, &ticket, 3, reducer); outcome != verdict {
			t.Fatalf("a branch answering %v settled the row as %v", verdict, outcome)
		}
		if len(reducer.seen) != 2 {
			t.Fatalf("the fold kept walking past a branch that ended the row: visited %d", len(reducer.seen))
		}
		// The row never publishes what it staged before the verdict.
		if !run.Submit(&ticket, verdict) {
			t.Fatalf("submit %v", verdict)
		}
		if _, _, branches, _ := run.Consume(); len(branches) != 0 {
			t.Fatalf("a row ended by %v published %d branches", verdict, len(branches))
		}
	}
}

// TestAnUndeclaredDispositionRefuses states the closed vocabulary: a reducer
// answering outside it has said nothing the row can settle on.
func TestAnUndeclaredDispositionRefuses(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	reducer := &branchLawReducer{verdicts: map[uint64]structure.ReductionOutcome{0: structure.ReductionOutcome(0)}}
	if outcome := FoldBranchSet(run, &ticket, 1, reducer); outcome != structure.Refuse {
		t.Fatalf("an undeclared disposition settled %v", outcome)
	}
}

// TestATriggerWithNoBranchSetAsksItsReducer states who answers for a trigger
// that declares no route at all - and that the answer may not claim to have
// settled branches there were none of.
//
// A negative census is not an empty one: it is a count nothing enumerated.
func TestATriggerWithNoBranchSetAsksItsReducer(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	empty := &branchLawReducer{empty: structure.NoCandidate}
	if outcome := FoldBranchSet(run, &ticket, 0, empty); outcome != structure.NoCandidate {
		t.Fatalf("an empty branch set settled %v, want the reducer's own answer", outcome)
	}
	claiming := &branchLawReducer{empty: structure.Concrete}
	if outcome := FoldBranchSet(run, &ticket, 0, claiming); outcome != structure.Refuse {
		t.Fatal("an empty branch set claimed to have settled a branch")
	}
	if outcome := FoldBranchSet(run, &ticket, -1, empty); outcome != structure.Refuse {
		t.Fatal("a census nothing enumerated was folded over")
	}
}

// TestAForeignTicketSettlesNoBranch keeps the invocation fence: the fold
// publishes through the Run that issued the ticket and no other.
func TestAForeignTicketSettlesNoBranch(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	other := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	reducer := &branchLawReducer{verdicts: map[uint64]structure.ReductionOutcome{0: structure.Concrete}}
	if outcome := FoldBranchSet(other, &ticket, 1, reducer); outcome != structure.Refuse {
		t.Fatalf("a foreign Run settled %v", outcome)
	}
	var absent Ticket
	if outcome := FoldBranchSet(run, &absent, 1, reducer); outcome != structure.Refuse {
		t.Fatalf("a zero ticket settled %v", outcome)
	}
}
