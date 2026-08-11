package observation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestSealReplacesEntireEquationObservationSet(t *testing.T) {
	guards, first, second := observationFixture(t)
	index := New(guards)
	coordinate := observationCoordinate(t)

	initial := index.Begin(NewEquation(3))
	if initial == nil || !initial.Read(coordinate, 7, 11, first) || !initial.Seal() {
		t.Fatal("initial observation did not seal")
	}
	replacement := index.Begin(NewEquation(3))
	if replacement == nil || !replacement.Plane(coordinate, 7, second) || !replacement.Seal() {
		t.Fatal("replacement observation did not seal")
	}
	if readers := index.keyed[unitID{source: coordinate, factor: 7, key: 11}]; len(readers) != 0 {
		t.Fatal("stale keyed observation survived replacement")
	}
	if readers := index.planes[planeID{source: coordinate, factor: 7}]; len(readers) != 1 {
		t.Fatalf("plane readers=%d, want one current reader", len(readers))
	}
	if _, present := index.equations[NewEquation(3)]; !present {
		t.Fatal("replacement erased current equation projection")
	}
}

func TestDiscardDoesNotReplaceCurrentEquationObservationSet(t *testing.T) {
	guards, first, second := observationFixture(t)
	index := New(guards)
	coordinate := observationCoordinate(t)
	committed := index.Begin(NewEquation(1))
	if committed == nil || !committed.Read(coordinate, 2, 5, first) || !committed.Seal() {
		t.Fatal("initial observation did not seal")
	}
	candidate := index.Begin(NewEquation(1))
	if candidate == nil || !candidate.Plane(coordinate, 2, second) {
		t.Fatal("candidate observation did not record")
	}
	candidate.Discard()
	if readers := index.keyed[unitID{source: coordinate, factor: 2, key: 5}]; len(readers) != 1 {
		t.Fatal("Discard replaced published observation")
	}
	if readers := index.planes[planeID{source: coordinate, factor: 2}]; len(readers) != 0 {
		t.Fatal("Discard published candidate plane observation")
	}
}

func TestExactUnitMasksUnionAndDefaultReadIsObserved(t *testing.T) {
	guards, first, second := observationFixture(t)
	index := New(guards)
	coordinate := observationCoordinate(t)
	log := index.Begin(NewEquation(9))
	// Read has no presence argument: an absent key's declared Default is an
	// observation and must retain the same exact invalidation unit.
	if log == nil || !log.Read(coordinate, 4, 42, first) || !log.Read(coordinate, 4, 42, second) || !log.Seal() {
		t.Fatal("default-capable duplicate direct read did not seal")
	}
	readers := index.keyed[unitID{source: coordinate, factor: 4, key: 42}]
	if len(readers) != 1 {
		t.Fatalf("deduplicated readers=%d, want one", len(readers))
	}
	region := readers[NewEquation(9)]
	if !region.Matches(func(atom guard.Atom) bool { return atom == 1 }) || !region.Matches(func(atom guard.Atom) bool { return atom != 1 }) {
		t.Fatal("duplicate observation masks were not exact-OR unioned")
	}
}

func TestDispatchSeparatesKeyPlaneAndSupportWithExactCompatibility(t *testing.T) {
	guards, first, second := observationFixture(t)
	all, ok := support.True(guards)
	if !ok {
		t.Fatal("true support")
	}
	index := New(guards)
	coordinate := observationCoordinate(t)
	if log := index.Begin(NewEquation(1)); log == nil || !log.Read(coordinate, 3, 8, first) || !log.Seal() {
		t.Fatal("key observation")
	}
	if log := index.Begin(NewEquation(2)); log == nil || !log.Plane(coordinate, 3, second) || !log.Seal() {
		t.Fatal("plane observation")
	}
	if log := index.Begin(NewEquation(3)); log == nil || !log.Read(coordinate, 3, 9, first) || !log.Seal() {
		t.Fatal("second keyed observation")
	}

	assertWake := func(change facts.Delta, want ...Equation) {
		t.Helper()
		var got []Equation
		seen := make(map[Equation]struct{})
		if !index.Dispatch(coordinate, change, func(equation Equation) bool {
			if _, duplicate := seen[equation]; duplicate {
				t.Fatalf("equation %d scheduled twice by one Delta", equation)
			}
			seen[equation] = struct{}{}
			got = append(got, equation)
			return true
		}) {
			t.Fatal("Dispatch rejected valid facts Delta")
		}
		if len(got) != len(want) {
			t.Fatalf("wake count=%v, want %v", got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("wake=%v, want %v", got, want)
			}
		}
	}

	// A key event reaches only the exact direct-key reader, and only under
	// an overlapping symbolic row.
	assertWake(facts.Delta{Kind: facts.DeltaKey, Region: first, Plane: 3, Key: 8}, NewEquation(1))
	assertWake(facts.Delta{Kind: facts.DeltaKey, Region: second, Plane: 3, Key: 8})
	// A Carry is a plane read, not a synthetic enumeration of key reads.
	assertWake(facts.Delta{Kind: facts.DeltaPlane, Region: second, Plane: 3}, NewEquation(2))
	assertWake(facts.Delta{Kind: facts.DeltaPlane, Region: first, Plane: 3})
	// Support addition/removal invalidates every compatible reader at this
	// coordinate, whatever direct Factor unit produced the outer change.
	assertWake(facts.Delta{Kind: facts.DeltaSupport, Support: first}, NewEquation(1), NewEquation(3))
	assertWake(facts.Delta{Kind: facts.DeltaSupport, Support: second}, NewEquation(2))
	assertWake(facts.Delta{Kind: facts.DeltaSupport, Support: all}, NewEquation(1), NewEquation(2), NewEquation(3))
}

func observationFixture(t *testing.T) (*guard.Manager, support.Mask, support.Mask) {
	t.Helper()
	guards, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	work := support.New(guards)
	if work == nil {
		t.Fatal("support work")
	}
	first, ok := work.Literal(1, false)
	if !ok {
		t.Fatal("first literal")
	}
	second, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		t.Fatal("second literal")
	}
	return guards, first, second
}

func observationCoordinate(t *testing.T) coordinate.Coordinate {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "observation", Text: []byte("return 0")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := source.ShardAt(0)
	if !ok {
		t.Fatal("shard")
	}
	table, ok := coordinate.New(source)
	if !ok {
		t.Fatal("coordinate table")
	}
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("entry")
	}
	result, ok := table.InternRoot(shard, entry)
	if !ok {
		t.Fatal("coordinate")
	}
	return result
}
