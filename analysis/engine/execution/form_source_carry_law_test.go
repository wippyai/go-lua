package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// sourceCarryLawReducer answers from its candidate alone. It takes no cell,
// because a rule of this form declares no read to deliver one.
type sourceCarryLawReducer struct {
	fact    uint64
	outcome structure.ReductionOutcome
}

func (reducer sourceCarryLawReducer) Reduce() (uint64, structure.ReductionOutcome) {
	return reducer.fact, reducer.outcome
}

// TestAReadFreeCarryPublishesTheRowAndAgesTheCarriedFacts states what the
// read-free WT form does. The row publishes the fact its candidate alone
// decides, at its own coordinate, and every carried coordinate reaches the
// successor state through the owner's map. Both land in one patch, so a reader
// never sees a state where one happened and the other did not.
//
// It is the carry form without a cell, not a source column: a source column is
// a materialization its owner sealed and carries nothing, while this row makes
// a judgment and carries.
func TestAReadFreeCarryPublishesTheRowAndAgesTheCarriedFacts(t *testing.T) {
	fixture := newCarryFixture(t)
	fixture.publish(t, fixture.targets[0], 7)
	fixture.publish(t, fixture.targets[1], 5)

	write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
	if !writeOK {
		t.Fatal("sealed read-free carry row")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue read-free carry invocation")
	}
	var writes Scratch[uint64, uint64]
	outcome := FoldSourceCarry(ticket, sourceCarryLawReducer{fact: 8, outcome: structure.Concrete}, write, &writes)
	if outcome != structure.Concrete {
		t.Fatalf("read-free carry fold = %v, want Concrete", outcome)
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit read-free carry invocation")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatalf("read-free carry drain = %v/%d/%t, want one patch", disposition, count, drained)
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("commit read-free carry patch")
	}
	fixture.state = next

	if value, present := fixture.observe(t, fixture.units[0]); !present || value != 8 {
		t.Fatalf("row coordinate = %d/%t, want the candidate's own 8", value, present)
	}
	if value, present := fixture.observe(t, fixture.units[1]); !present || value != 105 {
		t.Fatalf("carried coordinate = %d/%t, want the aged 105", value, present)
	}
}

// TestAReadFreeCarryThatPublishesNothingCarriesNothing states the other half of
// the disposition: a row whose judgment does not conclude Concrete leaves the
// Factor exactly as its predecessor left it. The carry is part of the
// publication, not a side effect that survives its refusal.
func TestAReadFreeCarryThatPublishesNothingCarriesNothing(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome structure.ReductionOutcome
	}{
		{name: "no-candidate", outcome: structure.NoCandidate},
		{name: "no-selection", outcome: structure.NoSelection},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCarryFixture(t)
			fixture.publish(t, fixture.targets[0], 7)
			fixture.publish(t, fixture.targets[1], 5)
			write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
			if !writeOK {
				t.Fatal("sealed read-free carry row")
			}
			run := NewRun(1, 1)
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
			if !issued {
				t.Fatal("issue read-free carry invocation")
			}
			var writes Scratch[uint64, uint64]
			if outcome := FoldSourceCarry(ticket, sourceCarryLawReducer{fact: 8, outcome: testCase.outcome}, write, &writes); outcome != testCase.outcome {
				t.Fatalf("read-free carry fold = %v, want %v", outcome, testCase.outcome)
			}
			if !run.Submit(&ticket, testCase.outcome) {
				t.Fatal("submit read-free carry invocation")
			}
			patches := make([]carrier.Patch, 1)
			if disposition, count, drained := run.Drain(patches); !drained || disposition != testCase.outcome || count != 0 {
				t.Fatalf("read-free carry drain = %v/%d/%t, want no patch", disposition, count, drained)
			}
			if value, present := fixture.observe(t, fixture.units[1]); !present || value != 5 {
				t.Fatalf("carried coordinate = %d/%t, want the untouched 5", value, present)
			}
		})
	}
}

// TestAReadFreeCarryRefusesAnUnavailableJudgment holds the fold to the
// dispositions the structure vocabulary declares. A judgment that answers
// outside them has said nothing, and publishing on it would stage a fact no
// domain concluded.
func TestAReadFreeCarryRefusesAnUnavailableJudgment(t *testing.T) {
	fixture := newCarryFixture(t)
	write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
	if !writeOK {
		t.Fatal("sealed read-free carry row")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue read-free carry invocation")
	}
	var writes Scratch[uint64, uint64]
	if outcome := FoldSourceCarry(ticket, sourceCarryLawReducer{fact: 8}, write, &writes); outcome != structure.Refuse {
		t.Fatalf("read-free carry fold = %v, want Refuse", outcome)
	}
}

// TestAReadFreeCarryRefusesAnUnsealedWrite states that a publication needs a
// sealed carry write and a lane to stage through. Neither is recoverable at
// invocation time, so the fold refuses rather than publishing partially.
func TestAReadFreeCarryRefusesAnUnsealedWrite(t *testing.T) {
	fixture := newCarryFixture(t)
	write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
	if !writeOK {
		t.Fatal("sealed read-free carry row")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue read-free carry invocation")
	}
	var writes Scratch[uint64, uint64]
	reducer := sourceCarryLawReducer{fact: 8, outcome: structure.Concrete}
	if outcome := FoldSourceCarry(ticket, reducer, CarryWrite[uint64, uint64]{}, &writes); outcome != structure.Refuse {
		t.Fatalf("unsealed write = %v, want Refuse", outcome)
	}
	if outcome := FoldSourceCarry(ticket, reducer, write, nil); outcome != structure.Refuse {
		t.Fatalf("absent lane = %v, want Refuse", outcome)
	}
}
