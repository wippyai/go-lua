package execution

import (
	"testing"

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
