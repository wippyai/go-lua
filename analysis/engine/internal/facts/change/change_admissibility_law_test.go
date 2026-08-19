package change

import "testing"

// The admissibility predicate is fail-closed by construction. These are the
// vocabulary halves of Amendment 1's three guard laws: the producer halves
// live at the sites that compute the coverage split.

func TestUnclassifiedEvidenceIsNeverAdmitted(t *testing.T) {
	cases := []struct {
		name    string
		set     Set
		admits  bool
		unknown bool
	}{
		{name: "zero set", set: Set{}, admits: false, unknown: true},
		{name: "reasons without direction", set: Set{Reasons: SupportAdded}, admits: false, unknown: true},
		{name: "ascends without known", set: Set{Reasons: SupportAdded, Direction: Ascends}, admits: false, unknown: true},
		{name: "known ascent", set: Set{Reasons: SupportAdded, Direction: Known | Ascends}, admits: true, unknown: false},
		{name: "known with no movement", set: Set{Reasons: ChangedUnit, Direction: Known}, admits: true, unknown: false},
		{name: "known descent", set: Set{Reasons: SupportRemoved, Direction: Known | Descends}, admits: false, unknown: false},
		{name: "known ascent and descent", set: Set{Direction: Known | Ascends | Descends}, admits: false, unknown: false},
		{name: "descent without known", set: Set{Direction: Descends}, admits: false, unknown: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := item.set.Admits(); got != item.admits {
				t.Fatalf("Admits()=%v want %v for %+v", got, item.admits, item.set)
			}
			if got := item.set.Unknown(); got != item.unknown {
				t.Fatalf("Unknown()=%v want %v for %+v", got, item.unknown, item.set)
			}
		})
	}
}

func TestUnionKeepsKnownConjunctiveAndMovementDisjunctive(t *testing.T) {
	ascent := Set{Reasons: SupportAdded, Direction: Known | Ascends}
	descent := Set{Reasons: SupportRemoved, Direction: Known | Descends}
	unclassified := Set{Reasons: ChangedFactor}
	cases := []struct {
		name  string
		left  Set
		right Set
		want  Set
	}{
		{name: "two ascents stay an ascent", left: ascent, right: ascent, want: ascent},
		{
			name:  "ascent with descent keeps both movements",
			left:  ascent,
			right: descent,
			want:  Set{Reasons: SupportAdded | SupportRemoved, Direction: Known | Ascends | Descends},
		},
		{
			name:  "ascent with unclassified is unclassified",
			left:  ascent,
			right: unclassified,
			want:  Set{Reasons: SupportAdded | ChangedFactor, Direction: Ascends},
		},
		{
			name:  "zero operand erases the classification",
			left:  ascent,
			right: Set{},
			want:  Set{Reasons: SupportAdded, Direction: Ascends},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := item.left.Union(item.right)
			if got != item.want {
				t.Fatalf("Union=%+v want %+v", got, item.want)
			}
			if commuted := item.right.Union(item.left); commuted != got {
				t.Fatalf("Union is not commutative: %+v vs %+v", commuted, got)
			}
		})
	}
}

// An accumulator that folds any unclassified operand into its evidence must
// refuse reuse, whatever order the operands arrive in.
func TestAccumulatedEvidenceRefusesReuseAfterAnyUnknownOperand(t *testing.T) {
	operands := []Set{
		{Reasons: SupportAdded, Direction: Known | Ascends},
		{Reasons: ChangedUnit, Direction: Known},
		{Reasons: AuthorshipChanged},
	}
	for skipped := range operands {
		accumulated := Set{Reasons: SupportAdded, Direction: Known | Ascends}
		unknownSeen := false
		for index, operand := range operands {
			if index == skipped {
				continue
			}
			unknownSeen = unknownSeen || operand.Unknown()
			accumulated = accumulated.Union(operand)
		}
		if accumulated.Admits() == unknownSeen {
			t.Fatalf("skipping operand %d: Admits()=%v with unknownSeen=%v (%+v)", skipped, accumulated.Admits(), unknownSeen, accumulated)
		}
	}
}

func TestReasonHistogramPositionsAreClosed(t *testing.T) {
	reasons := []Reason{ChangedUnit, ChangedFactor, SupportAdded, SupportRemoved, AuthorshipChanged}
	if ReasonWidth != len(reasons) {
		t.Fatalf("ReasonWidth=%d want %d", ReasonWidth, len(reasons))
	}
	for position, reason := range reasons {
		named, ok := ReasonAt(position)
		if !ok || named != reason {
			t.Fatalf("ReasonAt(%d)=(%d,%v) want (%d,true)", position, named, ok, reason)
		}
		if named&(named-1) != 0 {
			t.Fatalf("reason at position %d is not a single bit: %d", position, named)
		}
	}
	for _, outside := range []int{-1, len(reasons), len(reasons) + 1} {
		if _, ok := ReasonAt(outside); ok {
			t.Fatalf("ReasonAt(%d) admitted a position outside the closed vocabulary", outside)
		}
	}
}

func TestReasonBitsAccumulate(t *testing.T) {
	set := Set{}
	if set.Has(ChangedUnit) {
		t.Fatal("empty set has a reason")
	}
	set = set.With(ChangedUnit).With(SupportAdded)
	if !set.Has(ChangedUnit) || !set.Has(SupportAdded) || !set.Has(ChangedUnit|SupportAdded) {
		t.Fatalf("accumulated reasons lost: %+v", set)
	}
	if set.Has(ChangedFactor) {
		t.Fatalf("unaccumulated reason present: %+v", set)
	}
	if set.Empty() {
		t.Fatal("a set carrying reasons reports empty")
	}
	if !(Set{}).Empty() {
		t.Fatal("the zero set does not report empty")
	}
}
