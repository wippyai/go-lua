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
			disposition, patches, _, drained := run.Consume()
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

// TestExecutionWarmInvocationAllocatesOnlyAcceptedChange states the cost of
// one generated invocation. A generated member's whole dispatch is the
// sequence below - issue a ticket against the sealed catalog row, frame it,
// execute the typed family, drain the patches - and it runs once per member
// per solver round.
//
// Issue, frame, drain and the staging transaction itself are free: the
// candidate terminal page, the candidate FDD, the Boolean shell, both patch
// wrappers, the KeyChanges handed to expandChanges and expandChanges' own
// heap/unit/region scratch are all reusable storage grown once per worker and
// reset per write (profiled through factbinding.Binding.BeginInto ->
// facts/stage.BeginInto -> facts/diagram.BeginWithTerminalsInto ->
// facts/terminal.Arena.BeginInto, and through
// factbinding.(*Patch).Accept -> expandChanges).
//
// What remains is not fold overhead: it is the accepted change itself, which
// the carrier and the solver retain past this call, so it cannot be drawn
// from a lane-reset scratch without aliasing a patch still live in the round.
// Reusing it would be exactly the forbidden compensation this law would
// otherwise be gamed with. Every remaining allocation is one of:
//
//   - carrier.Issuer.IssueChange's changeRecord - the ChangeHandle backing
//     the accepted change identity Run.outputs and, later, State.Commit hold
//     onto.
//   - carrier.Work.AcceptAuthoredRows's owned []TargetRegion copy - the
//     authored row content of the accepted patch.
//
// Both read forms above pay exactly this floor (2): the source row also
// mints a brand-new root, so it pays four more retained pieces:
//
//   - IssueChange's independent copies of the units/regions evidence
//     (expandChanges' own scratch is reset on the next call, so the
//     changeRecord must own its copy).
//   - bindingWork.prepareChange's pendingRoot publisher, wrapping the new
//     semantic plane until the carrier resolves and commits that root.
//   - the four facts/diagram FDD nodes (the written terminal, the carried
//     terminal, the factor node, the key node) that record the newly
//     introduced fact and persist as part of the diagram thereafter.
//
// source's floor is 9, exact's is 2. A regression above either number is a
// new allocation to trace and fix; a claimed reduction must name which of the
// above stops being retained, not merely reuse a buffer under it (2026-08-24,
// CX-43).
func TestExecutionWarmInvocationAllocatesOnlyAcceptedChange(t *testing.T) {
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
			disposition, patches, _, drained := run.Consume()
			return executed && drained && disposition == structure.Concrete && len(patches) == result.Count()
		}
		measureWarmInvocation(t, invoke, 9)
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
			disposition, patches, _, drained := run.Consume()
			return executed && drained && disposition == structure.Concrete && len(patches) == result.Count()
		}
		measureWarmInvocation(t, invoke, 2)
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

// measureWarmInvocation asserts a warm invocation allocates exactly budget
// times - the accepted-change floor named on
// TestExecutionWarmInvocationAllocatesOnlyAcceptedChange, never more and
// never fewer, so both a new fold-overhead regression and an unnoticed change
// to what the carrier retains are caught.
func measureWarmInvocation(t *testing.T, invoke func() bool, budget float64) {
	t.Helper()
	if !invoke() || !invoke() {
		t.Fatal("warmup invocation")
	}
	allocations := testing.AllocsPerRun(50, func() {
		if !invoke() {
			t.Fatal("warm invocation")
		}
	})
	if allocations != budget {
		t.Fatalf("warm generated invocation allocated %v times, want %v", allocations, budget)
	}
}
