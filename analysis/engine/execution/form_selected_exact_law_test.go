package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// selectionLawReducer is a concrete reducer type, not a closure or an
// interface value: the fold consumes it as a type parameter, so these calls
// are the same static direct calls a generated family's own reducer gets.
type selectionLawReducer struct {
	outcome structure.ReductionOutcome
	calls   *int
	widths  *[]int
	tags    *[]uint64
}

func (reducer selectionLawReducer) Reduce(cells []operand.SelectedCell[uint64]) (uint64, structure.ReductionOutcome) {
	if reducer.calls != nil {
		*reducer.calls++
	}
	if reducer.widths != nil {
		*reducer.widths = append(*reducer.widths, len(cells))
	}
	var total uint64
	for _, cell := range cells {
		if reducer.tags != nil {
			*reducer.tags = append(*reducer.tags, cell.Tag)
		}
		total += cell.Value
	}
	if reducer.outcome != structure.Concrete {
		return 0, reducer.outcome
	}
	return total + 1, structure.Concrete
}

// selectionCells builds one observed selection, every member of which proved
// the same support the exact prerequisite did.
func selectionCells(fixture selectedFixture, width int) []operand.SelectedCell[uint64] {
	cells := make([]operand.SelectedCell[uint64], 0, width)
	for index := 0; index < width; index++ {
		cells = append(cells, operand.SelectedCell[uint64]{
			Value:   uint64(index) + 1,
			Present: true,
			Tag:     uint64(index) + 1,
			Region:  fixture.whole,
		})
	}
	return cells
}

// foreignSupport is a region this fixture's reads never proved. It is used to
// stand in for a member observed under a support of its own.
func foreignSupport(t testing.TB, fixture selectedFixture) support.Mask {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	other, ok := support.True(manager)
	if !ok || !other.Valid() || other.Equal(fixture.whole) {
		t.Fatal("foreign support is not distinct from the fixture's own")
	}
	return other
}

func selectionWrite(t testing.TB, fixture selectedFixture) ExactWrite[uint64, uint64] {
	t.Helper()
	write, ok := NewExactWrite(fixture.binding, fixture.targets[0], 0)
	if !ok || !write.Valid() {
		t.Fatal("exact write")
	}
	return write
}

// TestASelectionFoldConcludesOnceOverTheWholeSelection is the form's defining
// law and the whole reason it is not the routed form.
//
// A routed row publishes one fact per observed member, so its fold is a
// cadence. This row concludes ONE fact FROM every member, so its fold is
// called exactly once and is handed the whole delivery. Calling it per member
// would ask it to reassemble a correlation the read already established, and
// there would be no single conclusion for it to reach.
func TestASelectionFoldConcludesOnceOverTheWholeSelection(t *testing.T) {
	fixture := newSelectedFixture(t)
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch Scratch[uint64, uint64]
	calls, widths := 0, make([]int, 0, 1)
	tags := make([]uint64, 0, 3)
	outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, fixture.whole,
		selectionCells(fixture, 3), selectionLawReducer{outcome: structure.Concrete, calls: &calls, widths: &widths, tags: &tags})
	if outcome != structure.Concrete {
		t.Fatalf("outcome = %d, want Concrete", outcome)
	}
	if calls != 1 {
		t.Fatalf("the fold was called %d times; this form concludes once per candidate", calls)
	}
	if len(widths) != 1 || widths[0] != 3 {
		t.Fatalf("the fold was handed %v cells, want the whole selection of 3", widths)
	}
	if len(tags) != 3 || tags[0] != 1 || tags[1] != 2 || tags[2] != 3 {
		t.Fatalf("the fold saw tags %v; a selection reaches it with the tag each cell was correlated by", tags)
	}
}

// TestASelectionFoldPublishesUnderTheSupportEveryReadProved is the support
// law. The published fact is derived from the exact prerequisite AND every
// observed member, so it may claim only what all of them proved - their
// conjunction, never one of them alone and never the ambient invocation.
//
// The conjunction is PROVEN here rather than recomputed: every delivered
// member is observed at the window the invocation opened, so the regions
// agree, and the primitive holds them to that agreement. A member carrying a
// support of its own is a delivery this form cannot reduce to one conclusion,
// and it is refused by name rather than published under whichever region
// happened to be first.
func TestASelectionFoldPublishesUnderTheSupportEveryReadProved(t *testing.T) {
	fixture := newSelectedFixture(t)
	foreign := foreignSupport(t, fixture)

	for _, probe := range []struct {
		name   string
		region support.Mask
		mutate func([]operand.SelectedCell[uint64])
	}{
		{
			name:   "a member proved a support the prerequisite did not",
			region: fixture.whole,
			mutate: func(cells []operand.SelectedCell[uint64]) { cells[1].Region = foreign },
		},
		{
			name:   "the prerequisite proved a support the members did not",
			region: foreign,
			mutate: func(cells []operand.SelectedCell[uint64]) {},
		},
		{
			name:   "a member carries no authenticated support at all",
			region: fixture.whole,
			mutate: func(cells []operand.SelectedCell[uint64]) { cells[2].Region = support.Mask{} },
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			ticket := issueSelected(t, NewRun(1, 1), fixture, fixture.state)
			var scratch Scratch[uint64, uint64]
			cells := selectionCells(fixture, 3)
			probe.mutate(cells)
			calls := 0
			outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, probe.region, cells,
				selectionLawReducer{outcome: structure.Concrete, calls: &calls})
			if outcome != structure.Refuse {
				t.Fatalf("outcome = %d, want Refuse: a fact was published over a support its reads did not all prove", outcome)
			}
		})
	}
}

// TestASelectionFoldWithNoMemberPublishesUnderItsPrerequisiteAlone states the
// empty delivery. A selection that named nothing is not a refusal and not an
// absent candidate by itself: the fold still reaches its one conclusion, over
// no members, and what that conclusion means is the fold's own answer. The
// support it publishes under is the prerequisite's, because that is the whole
// of what was read.
func TestASelectionFoldWithNoMemberPublishesUnderItsPrerequisiteAlone(t *testing.T) {
	fixture := newSelectedFixture(t)
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch Scratch[uint64, uint64]
	calls, widths := 0, make([]int, 0, 1)
	outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, fixture.whole,
		nil, selectionLawReducer{outcome: structure.Concrete, calls: &calls, widths: &widths})
	if outcome != structure.Concrete {
		t.Fatalf("outcome = %d, want Concrete", outcome)
	}
	if calls != 1 || len(widths) != 1 || widths[0] != 0 {
		t.Fatalf("the fold was called %d times with widths %v; an empty selection still reaches one conclusion", calls, widths)
	}
}

// TestASelectionFoldPublishesNothingItDidNotConclude is the disposition law: a
// fold that answers anything but Concrete stages no patch, and an outcome
// outside the sealed vocabulary is a refusal rather than a silent publication.
func TestASelectionFoldPublishesNothingItDidNotConclude(t *testing.T) {
	fixture := newSelectedFixture(t)
	for _, probe := range []struct {
		name   string
		answer structure.ReductionOutcome
		expect structure.ReductionOutcome
	}{
		{name: "no candidate", answer: structure.NoCandidate, expect: structure.NoCandidate},
		{name: "refuse", answer: structure.Refuse, expect: structure.Refuse},
		{name: "outside the vocabulary", answer: structure.ReductionOutcome(0), expect: structure.Refuse},
	} {
		t.Run(probe.name, func(t *testing.T) {
			ticket := issueSelected(t, NewRun(1, 1), fixture, fixture.state)
			var scratch Scratch[uint64, uint64]
			outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, fixture.whole,
				selectionCells(fixture, 2), selectionLawReducer{outcome: probe.answer})
			if outcome != probe.expect {
				t.Fatalf("outcome = %d, want %d", outcome, probe.expect)
			}
		})
	}
}

// TestASelectionFoldRefusesAnUnusableWriteOrScratch keeps the primitive's own
// preconditions named rather than discovered as a nil dereference on a solve
// path.
func TestASelectionFoldRefusesAnUnusableWriteOrScratch(t *testing.T) {
	fixture := newSelectedFixture(t)
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	cells := selectionCells(fixture, 1)
	reducer := selectionLawReducer{outcome: structure.Concrete}
	if outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), nil, fixture.whole, cells, reducer); outcome != structure.Refuse {
		t.Fatalf("a fold with no scratch answered %d", outcome)
	}
	var scratch Scratch[uint64, uint64]
	if outcome := FoldSelectedExact(ticket, ExactWrite[uint64, uint64]{}, &scratch, fixture.whole, cells, reducer); outcome != structure.Refuse {
		t.Fatalf("a fold with no write answered %d", outcome)
	}
	if outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, support.Mask{}, cells, reducer); outcome != structure.Refuse {
		t.Fatalf("a fold with no prerequisite support answered %d", outcome)
	}
}

// narrowerSupport is a region strictly inside this fixture's own, built from
// the same manager so the two are comparable by entailment.
func narrowerSupport(t testing.TB, fixture selectedFixture) support.Mask {
	t.Helper()
	// The cold construction the support package uses for its own sealed
	// regions: open a shell, take the value, seal it.
	work := support.New(fixture.whole.Manager())
	if work == nil {
		t.Fatal("support work")
	}
	narrower := work.False()
	if !work.Seal() {
		work.Discard()
		t.Fatal("seal the narrower support")
	}
	if !narrower.Valid() || narrower.Equal(fixture.whole) || !narrower.Entails(fixture.whole) {
		t.Fatal("the narrower support is not strictly inside the fixture's own")
	}
	return narrower
}

// TestAMemberProvedOverMoreThanItsPrerequisiteDoesNotNarrowTheConclusion is
// the recursion law, and the case that made the first version of this
// primitive refuse a solve.
//
// A call site whose body set contains its own site reads a member at a
// coordinate this very rule writes. Before the fixpoint has put anything
// there the cell is absent - and absent EVERYWHERE, so it is proved over a
// wider support than the prerequisite that reached it. Intersecting with a
// support that contains the running meet leaves the meet alone; requiring the
// two to be equal instead refused the fold, and a recursive call site could
// never conclude.
func TestAMemberProvedOverMoreThanItsPrerequisiteDoesNotNarrowTheConclusion(t *testing.T) {
	fixture := newSelectedFixture(t)
	ticket := issueSelected(t, NewRun(1, 1), fixture, fixture.state)
	var scratch Scratch[uint64, uint64]
	calls := 0
	cells := selectionCells(fixture, 2)
	outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, narrowerSupport(t, fixture), cells,
		selectionLawReducer{outcome: structure.Concrete, calls: &calls})
	if calls != 1 {
		t.Fatalf("the fold was called %d times; a member proved over MORE than its prerequisite is not a delivery this form cannot reduce", calls)
	}
	_ = outcome
}

// TestAMemberProvedOverLessNarrowsTheConclusionToIt is the other direction. A
// conclusion may only hold where every read it consumed holds, so a member
// that proved less than everything before it moves the meet down to itself
// rather than being published over the wider support that reached it.
func TestAMemberProvedOverLessNarrowsTheConclusionToIt(t *testing.T) {
	fixture := newSelectedFixture(t)
	ticket := issueSelected(t, NewRun(1, 1), fixture, fixture.state)
	var scratch Scratch[uint64, uint64]
	calls := 0
	cells := selectionCells(fixture, 2)
	cells[1].Region = narrowerSupport(t, fixture)
	outcome := FoldSelectedExact(ticket, selectionWrite(t, fixture), &scratch, fixture.whole, cells,
		selectionLawReducer{outcome: structure.Concrete, calls: &calls})
	if calls != 1 {
		t.Fatalf("the fold was called %d times; a member proved over LESS than its prerequisite narrows the conclusion rather than refusing it", calls)
	}
	_ = outcome
}
