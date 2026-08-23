package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestExecutionSourceFamilyConcludesTheSealedRowOutcome states what a zero-read
// source row answers: the disposition its owner sealed for that dense
// candidate. A materializer is total over the candidate directory, but not
// every candidate carries a fact - an owner-issued row may be a sealed absence
// - so the Z form must be able to conclude NoCandidate for a row that is
// nonetheless a real occurrence.
func TestExecutionSourceFamilyConcludesTheSealedRowOutcome(t *testing.T) {
	fixture := newExecutionFixture(t)
	// The invocation row this fixture issues names candidate ordinal 3.
	const candidate = 3
	for _, testCase := range []struct {
		name    string
		outcome structure.ReductionOutcome
		patches int
	}{
		{name: "concrete", outcome: structure.Concrete, patches: 1},
		{name: "no-candidate", outcome: structure.NoCandidate},
		{name: "no-selection", outcome: structure.NoSelection},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			values := []uint64{10, 11, 12, 13}
			outcomes := []structure.ReductionOutcome{
				structure.Concrete, structure.Concrete, structure.Concrete, testCase.outcome,
			}
			column, columnOK := memberrelation.NewSourceColumn(values, outcomes)
			if !columnOK {
				t.Fatal("sealed source column")
			}
			run := NewRun(0, 1)
			row, rowOK := NewSourceRow(fixture.binding, fixture.target, 0, column)
			if !rowOK {
				t.Fatal("sealed source row")
			}
			family, familyOK := NewSourceFamily([]SourceRow[uint64, uint64]{row})
			if !familyOK {
				t.Fatal("sealed source family")
			}
			executor := family.NewExecutor(run)
			ticket := issueExecution(t, run, fixture)
			if ordinal, ok := ticket.CandidateOrdinal(); !ok || ordinal != candidate {
				t.Fatalf("fixture candidate ordinal = %d/%t", ordinal, ok)
			}
			frame, frameOK := NewFrame(ticket)
			if executor == nil || !frameOK {
				t.Fatal("source frame")
			}
			result, executed := executor.Execute(frame, ticket)
			if !executed || !result.Valid() || result.Outcome() != testCase.outcome || result.Count() != testCase.patches {
				t.Fatalf("source result = %+v/%t, want outcome %v with %d patches", result, executed, testCase.outcome, testCase.patches)
			}
			disposition, patches, drained := run.Consume()
			if !drained || disposition != testCase.outcome || len(patches) != testCase.patches {
				t.Fatalf("source drain = %v/%d/%t", disposition, len(patches), drained)
			}
		})
	}
}

// TestSourceColumnIsOneRowTable is the nearest declaration negative: values and
// outcomes are one table, and a Refuse row is not an outcome an owner may seal
// - a materializer that declines to answer has no row to publish.
func TestSourceColumnIsOneRowTable(t *testing.T) {
	if _, ok := memberrelation.NewSourceColumn([]uint64{1, 2}, []structure.ReductionOutcome{structure.Concrete}); ok {
		t.Fatal("a short outcome table sealed a source column")
	}
	if _, ok := memberrelation.NewSourceColumn([]uint64{1}, []structure.ReductionOutcome{structure.Refuse}); ok {
		t.Fatal("a refusing row sealed a source column")
	}
	empty, emptyOK := memberrelation.NewSourceColumn[uint64](nil, nil)
	if !emptyOK || !empty.Valid() || empty.Count() != 0 {
		t.Fatalf("sealed empty column = %t/%t/%d", emptyOK, empty.Valid(), empty.Count())
	}
	if _, _, ok := (memberrelation.SourceColumn[uint64]{}).At(0); ok {
		t.Fatal("an unsealed column indexed a row")
	}
}

// TestExecutionWarmInvocationAllocatesNothing states the cost of one generated
// invocation. A generated member's whole dispatch is the sequence below -
// issue a ticket against the sealed catalog row, frame it, execute the typed
// family, drain the patches - and it runs once per member per solver round.
// A single allocation here is a per-member-per-round allocation, so the
// admitted budget is zero for both of the forms the generated arm currently
// executes: the zero-read source row and the one exact read that carries its
// own identity forward.
//
// This law is RED, and the cost is not in the dispatch. Issue, frame and drain
// are already free; every allocation comes from the staged write, where
// factbinding.Binding.Begin opens a fresh diagram builder and terminal arena
// for each staging transaction (27 objects / ~7KB per staged write, profiled
// through facts/stage.Begin -> facts/diagram.begin -> facts/terminal.Arena).
// That path is shared with every hand-wired rule, so the defect is the fact
// plane's staging transaction rather than the generated arm; it is stated here
// because this is where the generated arm pays it.
func TestExecutionWarmInvocationAllocatesNothing(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		fixture := newExecutionFixture(t)
		column, columnOK := memberrelation.NewSourceColumn(
			[]uint64{1, 2, 3, 4},
			[]structure.ReductionOutcome{structure.Concrete, structure.Concrete, structure.Concrete, structure.Concrete},
		)
		row, rowOK := NewSourceRow(fixture.binding, fixture.target, 0, column)
		if !columnOK || !rowOK {
			t.Fatal("sealed source row")
		}
		family, familyOK := NewSourceFamily([]SourceRow[uint64, uint64]{row})
		if !familyOK {
			t.Fatal("sealed source family")
		}
		run := NewRun(0, 1)
		executor := family.NewExecutor(run)
		if executor == nil {
			t.Fatal("source executor")
		}
		invoke := func() bool {
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
			frame, framed := NewFrame(ticket)
			if !issued || !framed {
				return false
			}
			result, executed := executor.Execute(frame, ticket)
			disposition, patches, drained := run.Consume()
			return executed && drained && disposition == structure.Concrete && len(patches) == result.Count()
		}
		measureWarmInvocation(t, invoke)
	})
	t.Run("exact", func(t *testing.T) {
		fixture := publishedExecutionFixture(t)
		row, rowOK := NewExactRow(fixture.binding, fixture.unit, 0, fixture.target, 0)
		family, familyOK := NewExactFamily([]ExactRow[uint64, uint64]{row})
		if !rowOK || !familyOK {
			t.Fatal("sealed exact family")
		}
		run := NewRun(1, 1)
		executor := family.NewExecutor(run)
		if executor == nil {
			t.Fatal("exact executor")
		}
		inputs := []carrier.State{fixture.state}
		invoke := func() bool {
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, 1, 4, 9, 2)
			frame, framed := NewFrame(ticket)
			if !issued || !framed {
				return false
			}
			result, executed := executor.Execute(frame, ticket)
			disposition, patches, drained := run.Consume()
			return executed && drained && disposition == structure.Concrete && len(patches) == result.Count()
		}
		measureWarmInvocation(t, invoke)
	})
}

// publishedExecutionFixture is the ordinary fixture with one committed value,
// so an exact read finds a present row and its fold concludes Concrete. A fold
// that concludes no candidate never reaches the staging path, which is where
// an invocation would allocate if it allocated at all.
func publishedExecutionFixture(t testing.TB) executionFixture {
	t.Helper()
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !writeOK || !issued {
		t.Fatal("published fixture write")
	}
	var scratch Scratch[uint64, uint64]
	patches := make([]carrier.Patch, 1)
	if !write.Stage(ticket, &scratch, fixture.whole, 7) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("published fixture stage")
	}
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("published fixture drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("published fixture commit")
	}
	fixture.state = next
	return fixture
}

func measureWarmInvocation(t *testing.T, invoke func() bool) {
	t.Helper()
	if !invoke() || !invoke() {
		t.Fatal("warmup invocation")
	}
	allocations := testing.AllocsPerRun(50, func() {
		if !invoke() {
			t.Fatal("warm invocation")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm generated invocation allocated %v times", allocations)
	}
}
