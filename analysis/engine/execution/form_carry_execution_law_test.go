package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// carryFixture is a two-coordinate Factor: the row writes the first, and the
// second is carried. One coordinate cannot state a carry law at all - the row
// write would overwrite whatever the transform produced.
type carryFixture struct {
	binding *factbinding.Binding[uint64, uint64]
	units   [2]carrier.Unit
	targets [2]carrier.Target
	state   carrier.State
	whole   support.Mask
	work    *carrier.Work
}

func newCarryFixture(t testing.TB) carryFixture {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	algebra, ok := factbinding.Admit[uint64, uint64](2, 0, lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return 0 },
		Equal:    func(left, right uint64) bool { return left == right },
		Same:     func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}, func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("algebra")
	}
	fixture := carryFixture{whole: whole}
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		// Every exact Unit is declared before any Target: the declaration phase
		// admits units first and this Factor has two coordinates.
		for key := uint64(0); key < 2; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			fixture.units[key] = unit
		}
		for key := uint64(0); key < 2; key++ {
			target, strong := binding.DeclareStrong(fixture.units[key])
			if !strong {
				return false
			}
			fixture.targets[key] = target
		}
		return true
	})
	if !ok {
		t.Fatal("binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	fixture.binding, fixture.state, fixture.work = binding, state, work
	return fixture
}

// publish commits one value at one coordinate so a later invocation observes it.
func (fixture *carryFixture) publish(t testing.TB, target carrier.Target, value uint64) {
	t.Helper()
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, target, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !writeOK || !issued {
		t.Fatal("publish write")
	}
	var scratch Scratch[uint64, uint64]
	patches := make([]carrier.Patch, 1)
	if !write.Stage(ticket, &scratch, fixture.whole, value) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("publish stage")
	}
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("publish commit")
	}
	fixture.state = next
}

// observe reads one committed coordinate back.
func (fixture *carryFixture) observe(t testing.TB, unit carrier.Unit) (uint64, bool) {
	t.Helper()
	run := NewRun(1, 1)
	read, readOK := NewExactRead(fixture.binding, unit, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !readOK || !issued {
		t.Fatal("observe read")
	}
	var scratch Scratch[uint64, uint64]
	if read.Read(ticket, &scratch) != ReadAvailable {
		t.Fatal("observe cursor")
	}
	value, valueOK := scratch.Value()
	present := scratch.Present()
	if !read.Close(ticket, &scratch) {
		t.Fatal("observe close")
	}
	_ = run.Submit(&ticket, structure.NoCandidate)
	_, _, _, _ = run.Consume()
	return value, valueOK && present
}

// carryLawReducer publishes the successor of the fact it read.
type carryLawReducer struct{ outcome structure.ReductionOutcome }

func (reducer carryLawReducer) Reduce(read uint64, present bool) (uint64, structure.ReductionOutcome) {
	if !present {
		return 0, structure.NoCandidate
	}
	return read + 1, reducer.outcome
}

// ageCarry is an owner-issued map that fixes the Factor default and ages every
// other fact.
func ageCarry(prior uint64) (uint64, bool) {
	if prior == 0 {
		return 0, true
	}
	return prior + 100, true
}

// TestATransformedCarryPublishesTheRowAndAgesTheCarriedFacts states what the WT
// form does that the identity fold cannot: the row publishes its own fact at
// its own coordinate, and every carried coordinate reaches the successor state
// through the owner's map rather than unchanged. Both land in one patch, so a
// reader never sees a state where one happened and the other did not.
func TestATransformedCarryPublishesTheRowAndAgesTheCarriedFacts(t *testing.T) {
	fixture := newCarryFixture(t)
	fixture.publish(t, fixture.targets[0], 7)
	fixture.publish(t, fixture.targets[1], 5)

	read, readOK := NewExactRead(fixture.binding, fixture.units[0], 0)
	write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
	if !readOK || !writeOK {
		t.Fatal("sealed carry row")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue carry invocation")
	}
	var reads, writes Scratch[uint64, uint64]
	outcome := FoldCarry(ticket, carryLawReducer{outcome: structure.Concrete}, read, &reads, write, &writes)
	if outcome != structure.Concrete {
		t.Fatalf("carry fold = %v, want Concrete", outcome)
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit carry invocation")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatalf("carry drain = %v/%d/%t, want one patch", disposition, count, drained)
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("commit carry patch")
	}
	fixture.state = next

	if value, present := fixture.observe(t, fixture.units[0]); !present || value != 8 {
		t.Fatalf("row coordinate = %d/%t, want the reduced 8", value, present)
	}
	if value, present := fixture.observe(t, fixture.units[1]); !present || value != 105 {
		t.Fatalf("carried coordinate = %d/%t, want the aged 105", value, present)
	}
}

// TestATransformedCarryThatPublishesNothingCarriesNothing states the other half
// of the disposition: a row whose reducer does not conclude Concrete leaves the
// Factor exactly as its predecessor left it. The carry is part of the
// publication, not a side effect that survives its refusal.
func TestATransformedCarryThatPublishesNothingCarriesNothing(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		outcome structure.ReductionOutcome
	}{
		{name: "no-candidate", outcome: structure.NoCandidate},
		{name: "no-selection", outcome: structure.NoSelection},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			outcome := testCase.outcome
			fixture := newCarryFixture(t)
			fixture.publish(t, fixture.targets[0], 7)
			fixture.publish(t, fixture.targets[1], 5)
			read, readOK := NewExactRead(fixture.binding, fixture.units[0], 0)
			write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
			if !readOK || !writeOK {
				t.Fatal("sealed carry row")
			}
			run := NewRun(1, 1)
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
			if !issued {
				t.Fatal("issue carry invocation")
			}
			var reads, writes Scratch[uint64, uint64]
			if folded := FoldCarry(ticket, carryLawReducer{outcome: outcome}, read, &reads, write, &writes); folded != outcome {
				t.Fatalf("carry fold = %v, want %v", folded, outcome)
			}
			if !run.Submit(&ticket, outcome) {
				t.Fatal("submit carry invocation")
			}
			disposition, patches, _, drained := run.Consume()
			if !drained || disposition != outcome || len(patches) != 0 {
				t.Fatalf("a refused row staged %d patches", len(patches))
			}
			if value, present := fixture.observe(t, fixture.units[1]); !present || value != 5 {
				t.Fatalf("carried coordinate = %d/%t, want the untouched 5", value, present)
			}
		})
	}
}

// TestACarryMapMustFixTheFactorDefault states the law a transformed carry owes
// its Factor: the map is applied to every coordinate of the carried closure,
// including the ones the Factor never wrote, so a map that moves the default
// invents a fact everywhere. It is refused where it is sealed, once, rather
// than being discovered one coordinate at a time.
func TestACarryMapMustFixTheFactorDefault(t *testing.T) {
	fixture := newCarryFixture(t)
	moves := func(prior uint64) (uint64, bool) { return prior + 1, true }
	if _, ok := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, moves); ok {
		t.Fatal("a carry map that moves the Factor default was sealed")
	}
	refuses := func(prior uint64) (uint64, bool) { return prior, false }
	if _, ok := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, refuses); ok {
		t.Fatal("a carry map that refuses the Factor default was sealed")
	}
	if _, ok := NewCarryWrite(fixture.binding, fixture.targets[0], 0, nil, ageCarry); ok {
		t.Fatal("a transformed carry with no carried coordinate was sealed")
	}
	if _, ok := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, nil); ok {
		t.Fatal("a transformed carry with no map was sealed")
	}
}

// TestAWarmTransformedCarryAllocatesNothingBeyondItsPublication holds the WT
// form to the budget of the forms beside it. A row that publishes nothing is
// the sharp statement: it issues, reads, reduces and concludes without opening
// a write transaction, so it must allocate nothing at all. A row that does
// publish pays only for the change it hands the solver - the transaction, the
// transform over the carried closure, and the staged row are all reusable
// storage, which the profile of the publishing case confirms.
func TestAWarmTransformedCarryAllocatesNothingBeyondItsPublication(t *testing.T) {
	fixture := newCarryFixture(t)
	fixture.publish(t, fixture.targets[0], 7)
	fixture.publish(t, fixture.targets[1], 5)
	read, readOK := NewExactRead(fixture.binding, fixture.units[0], 0)
	write, writeOK := NewCarryWrite(fixture.binding, fixture.targets[0], 0, []carrier.Target{fixture.targets[1]}, ageCarry)
	if !readOK || !writeOK {
		t.Fatal("sealed carry row")
	}
	run := NewRun(1, 1)
	var reads, writes Scratch[uint64, uint64]
	inputs := []carrier.State{fixture.state}
	invoke := func() bool {
		ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, 1, 4, 9, 2)
		if !issued {
			return false
		}
		outcome := FoldCarry(ticket, carryLawReducer{outcome: structure.NoSelection}, read, &reads, write, &writes)
		if !run.Submit(&ticket, outcome) {
			return false
		}
		disposition, patches, _, drained := run.Consume()
		return drained && disposition == structure.NoSelection && len(patches) == 0
	}
	measureWarmInvocation(t, invoke, 0)
}
