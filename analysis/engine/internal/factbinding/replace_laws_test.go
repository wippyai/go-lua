package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestReplaceInstallsExactRightRootsAndReportsOnlyOverlap(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	old := writeStates(t, newWork(t, composition), binding, fixture, base, slot, []factWrite{{when: on, value: 1}, {when: off, value: 11}})
	rightFull := writeStates(t, newWork(t, composition), binding, fixture, base, slot, []factWrite{{when: on, value: 2}, {when: off, value: 22}})
	rightView, ok := rightFull.Restrict(on)
	if !ok {
		t.Fatal("right view")
	}
	// Transfer is only used to create a valid recomputed state whose hidden
	// off-support meaning differs. Replace must install that root verbatim.
	recomputed, _, ok := newWork(t, composition).Transfer(rightFull, rightView, nil)
	if !ok {
		t.Fatal("restricted recomputed state")
	}
	work := newWork(t, composition)
	next, changes, ok := work.Replace(old, recomputed)
	if !ok || !next.Support().Equal(on) || !support.Empty(changes.Added()) || !changes.Removed().Equal(off) || changes.Count() != 1 || changes.FactorCount() != 1 {
		t.Fatalf("replace shape: ok=%t support=%t added=%t removed=%t rows=%d", ok, next.Support().Equal(on), support.Empty(changes.Added()), changes.Removed().Equal(off), changes.Count())
	}
	row, ok := changes.At(0)
	if !ok || !row.Region().Equal(on) {
		t.Fatal("replace did not report the exact overlap change")
	}
	factor, present := changes.FactorAt(0)
	if !present || factor.Slot() != slot || !factor.Region().Equal(on) {
		t.Fatal("replace did not report its exact overlap Factor region")
	}
	rightRoot, _ := recomputed.HandleAt(slot)
	nextRoot, _ := next.HandleAt(slot)
	if nextRoot != rightRoot {
		t.Fatal("replace retained or rebuilt a root instead of installing right")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), nextRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 22 {
		t.Fatalf("right hidden root = %d/%t/%t, want 22/true/true", got, present, valid)
	}
	if !work.EqualUnder(next, recomputed) {
		t.Fatal("replacement is not equal to recomputed under its exact support")
	}
	// Growing support later cannot surface old's off-support value 11 because
	// next already holds recomputed's exact root.
	grown, grownChanges, ok := work.Replace(next, rightFull)
	if !ok || !grownChanges.Added().Equal(off) || grownChanges.Count() != 0 || grownChanges.FactorCount() != 0 || !work.EqualUnder(grown, rightFull) {
		t.Fatal("support growth did not retain the replacement right root")
	}
}

func TestReplaceTreatsOneSidedSupportAsStructuralOnly(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), on)
	old := writeState(t, newWork(t, composition), binding, fixture, base, slot, on, 1)
	rightBase, ok := carrier.NewState(composition, composition.Scope(), off)
	if !ok {
		t.Fatal("right state")
	}
	right := writeState(t, newWork(t, composition), binding, fixture, rightBase, slot, off, 2)
	next, changes, ok := newWork(t, composition).Replace(old, right)
	if !ok || !next.Support().Equal(off) || !changes.Added().Equal(off) || !changes.Removed().Equal(on) || changes.Count() != 0 || changes.FactorCount() != 0 {
		t.Fatalf("one-sided replacement produced plane deltas: ok=%t rows=%d", ok, changes.Count())
	}
}

func TestReplaceMultiFactorRowsAreCanonicalAndSameStateIsAllocationFree(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first, firstFixture := newLawBinding(t, manager, false)
	second, secondFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	base, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("base")
	}
	old := writeState(t, newWork(t, composition), first, firstFixture, base, shape.Slot(0), whole, 1)
	old = writeState(t, newWork(t, composition), second, secondFixture, old, shape.Slot(1), whole, 1)
	right := writeState(t, newWork(t, composition), first, firstFixture, base, shape.Slot(0), whole, 2)
	right = writeState(t, newWork(t, composition), second, secondFixture, right, shape.Slot(1), whole, 3)
	work := newWork(t, composition)
	next, changes, ok := work.Replace(old, right)
	if !ok || !work.EqualUnder(next, right) || changes.Count() != 2 || changes.FactorCount() != 2 {
		t.Fatalf("multi-factor replacement: ok=%t equal=%t rows=%d", ok, work.EqualUnder(next, right), changes.Count())
	}
	previous := carrier.Unit{}
	for index := 0; index < changes.Count(); index++ {
		row, ok := changes.At(index)
		if !ok || index > 0 && !previous.Less(row.Unit()) || !row.Region().Equal(whole) {
			t.Fatal("replacement rows are not in canonical slot/unit order")
		}
		previous = row.Unit()
	}
	for index, slot := range []shape.Slot{0, 1} {
		factor, present := changes.FactorAt(index)
		if !present || factor.Slot() != slot || !factor.Region().Equal(whole) {
			t.Fatal("replacement Factor regions are not in canonical slot order")
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		unchanged, empty, replaced := work.Replace(next, next)
		if !replaced || !empty.Empty() || !work.EqualUnder(unchanged, next) {
			t.Fatal("same replacement")
		}
	}); allocations != 0 {
		t.Fatalf("same replacement allocations/run = %g, want 0", allocations)
	}
}
