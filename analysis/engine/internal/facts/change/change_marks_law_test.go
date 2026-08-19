package change

import (
	"reflect"
	"testing"
)

func TestMarksAccumulateEvidenceInFirstMarkOrder(t *testing.T) {
	cases := []struct {
		name  string
		width int
		marks []struct {
			ord Ord
			set Set
		}
		dirty []Ord
		at    map[Ord]Set
	}{
		{
			name:  "distinct ordinals keep arrival order",
			width: 8,
			marks: []struct {
				ord Ord
				set Set
			}{
				{ord: 5, set: Set{Reasons: SupportAdded, Direction: Known | Ascends}},
				{ord: 1, set: Set{Reasons: SupportRemoved, Direction: Known | Descends}},
				{ord: 7, set: Set{Reasons: ChangedUnit}},
			},
			dirty: []Ord{5, 1, 7},
			at: map[Ord]Set{
				5: {Reasons: SupportAdded, Direction: Known | Ascends},
				1: {Reasons: SupportRemoved, Direction: Known | Descends},
				7: {Reasons: ChangedUnit},
				0: {},
			},
		},
		{
			name:  "a repeat mark unions in place and keeps its first position",
			width: 4,
			marks: []struct {
				ord Ord
				set Set
			}{
				{ord: 2, set: Set{Reasons: SupportAdded, Direction: Known | Ascends}},
				{ord: 0, set: Set{Reasons: ChangedFactor, Direction: Known}},
				{ord: 2, set: Set{Reasons: AuthorshipChanged}},
			},
			dirty: []Ord{2, 0},
			at: map[Ord]Set{
				2: {Reasons: SupportAdded | AuthorshipChanged, Direction: Ascends},
				0: {Reasons: ChangedFactor, Direction: Known},
			},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var marks Marks
			marks.Reset(item.width)
			for _, mark := range item.marks {
				if !marks.Mark(mark.ord, mark.set) {
					t.Fatalf("Mark(%d) refused", mark.ord)
				}
			}
			if !reflect.DeepEqual(marks.Dirty(), item.dirty) {
				t.Fatalf("Dirty()=%v want %v", marks.Dirty(), item.dirty)
			}
			for ord, want := range item.at {
				if got := marks.At(ord); got != want {
					t.Fatalf("At(%d)=%+v want %+v", ord, got, want)
				}
			}
		})
	}
}

func TestMarksResetClearsTheVisitLayerAndKeepsTheSinceLayer(t *testing.T) {
	var marks Marks
	marks.Reset(4)
	first := Set{Reasons: SupportAdded, Direction: Known | Ascends}
	if !marks.Mark(3, first) {
		t.Fatal("Mark refused")
	}
	marks.Reset(4)
	if !marks.Empty() || len(marks.Dirty()) != 0 {
		t.Fatalf("Reset left %v live", marks.Dirty())
	}
	if got := marks.At(3); got != (Set{}) {
		t.Fatalf("At after Reset=%+v want zero", got)
	}
	if got := marks.Since(3); got != first {
		t.Fatalf("Since after Reset=%+v want %+v", got, first)
	}
	second := Set{Reasons: ChangedFactor, Direction: Known}
	if !marks.Mark(3, second) {
		t.Fatal("Mark refused")
	}
	if got, want := marks.Since(3), first.Union(second); got != want {
		t.Fatalf("Since accumulated=%+v want %+v", got, want)
	}
	if got := marks.At(3); got != second {
		t.Fatalf("At=%+v want %+v", got, second)
	}
	marks.Remember()
	if got := marks.Since(3); got != (Set{}) {
		t.Fatalf("Since after Remember=%+v want zero", got)
	}
	if got := marks.At(3); got != second {
		t.Fatalf("Remember disturbed the visit layer: %+v", got)
	}
	marks.Mark(1, first)
	marks.Invalidate()
	if got := marks.Since(1); got != (Set{}) {
		t.Fatalf("Since after Invalidate=%+v want zero", got)
	}
}

func TestMarksRefuseOrdinalsOutsideThePlane(t *testing.T) {
	var marks Marks
	marks.Reset(2)
	for _, ord := range []Ord{2, 9, NoOrd} {
		if marks.Mark(ord, Set{Reasons: ChangedUnit, Direction: Known}) {
			t.Fatalf("Mark(%d) admitted on a width-2 plane", ord)
		}
		if got := marks.At(ord); got != (Set{}) {
			t.Fatalf("At(%d)=%+v want zero", ord, got)
		}
		if got := marks.Since(ord); got != (Set{}) {
			t.Fatalf("Since(%d)=%+v want zero", ord, got)
		}
	}
	if !marks.Empty() {
		t.Fatalf("refused marks became live: %v", marks.Dirty())
	}
}

func TestMarksWidenWithoutClearingRetainedHistory(t *testing.T) {
	var marks Marks
	marks.Reset(2)
	retained := Set{Reasons: SupportAdded, Direction: Known | Ascends}
	marks.Mark(1, retained)
	marks.Reset(6)
	if marks.Width() != 6 {
		t.Fatalf("Width()=%d want 6", marks.Width())
	}
	if got := marks.Since(1); got != retained {
		t.Fatalf("widening lost since evidence: %+v", got)
	}
	for _, ord := range []Ord{2, 5} {
		if got := marks.Since(ord); got != (Set{}) {
			t.Fatalf("appended ordinal %d reads as marked: %+v", ord, got)
		}
		if !marks.Mark(ord, retained) {
			t.Fatalf("Mark(%d) refused after widening", ord)
		}
	}
}

// A wrapped stamp epoch is the only case that pays a real clear, and it must
// not resurrect the marks of the epoch it wrapped out of.
func TestMarksWrapClearsInsteadOfAliasingAnOldEpoch(t *testing.T) {
	var marks Marks
	marks.Reset(3)
	marks.Mark(2, Set{Reasons: ChangedUnit, Direction: Known})
	marks.visitMark = ^uint32(0)
	marks.visitStamp[2] = ^uint32(0)
	marks.Reset(3)
	if got := marks.At(2); got != (Set{}) {
		t.Fatalf("wrapped epoch aliased a stale mark: %+v", got)
	}
	if marks.visitMark != firstMark {
		t.Fatalf("visitMark=%d want %d after wrap", marks.visitMark, firstMark)
	}
	if !marks.Mark(2, Set{Reasons: SupportAdded, Direction: Known | Ascends}) {
		t.Fatal("Mark refused after wrap")
	}
	if got := marks.At(2); (got != Set{Reasons: SupportAdded, Direction: Known | Ascends}) {
		t.Fatalf("post-wrap mark lost: %+v", got)
	}
}

func TestNilMarksRefuseEverything(t *testing.T) {
	var marks *Marks
	marks.Reset(4)
	if marks.Mark(0, Set{Direction: Known}) {
		t.Fatal("a nil accumulator admitted a mark")
	}
	if got := marks.At(0); got != (Set{}) {
		t.Fatalf("At=%+v want zero", got)
	}
	if got := marks.Since(0); got != (Set{}) {
		t.Fatalf("Since=%+v want zero", got)
	}
	if !marks.Empty() || marks.Dirty() != nil || marks.Width() != 0 {
		t.Fatal("a nil accumulator reported state")
	}
	marks.Remember()
}

func TestMarkedSeparatesTheFirstMarkFromAnAccumulatingOne(t *testing.T) {
	var marks Marks
	marks.Reset(3)
	if marks.Marked(1) {
		t.Fatal("an unmarked ordinal reports marked")
	}
	marks.Mark(1, Set{Reasons: SupportAdded, Direction: Known | Ascends})
	if !marks.Marked(1) {
		t.Fatal("a marked ordinal reports unmarked")
	}
	if marks.Marked(0) || marks.Marked(3) || marks.Marked(NoOrd) {
		t.Fatal("Marked admitted an unmarked or out-of-plane ordinal")
	}
	marks.Reset(3)
	if marks.Marked(1) {
		t.Fatal("Reset left an ordinal marked")
	}
	var nilMarks *Marks
	if nilMarks.Marked(0) {
		t.Fatal("a nil accumulator reports a mark")
	}
}
