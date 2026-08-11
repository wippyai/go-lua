package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func acceptedTransferWrite(t testing.TB, binding *Binding[uint64, uint64], work *carrier.Work, predecessor carrier.State, target carrier.Target, when support.Mask, value uint64) carrier.Patch {
	t.Helper()
	staged := binding.Begin(work, predecessor)
	if staged == nil || !staged.Write(target, when, value) {
		t.Fatal("stage transfer patch")
	}
	patch, ok := staged.Accept(work)
	if !ok {
		t.Fatal("accept transfer patch")
	}
	return patch
}

func TestTransferBindsExactRestrictedPredecessorAndMask(t *testing.T) {
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
	binding, predecessor, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	target := fixture.target(t, 0, carrier.StrongTarget)
	view, ok := predecessor.Restrict(on)
	if !ok {
		t.Fatal("restricted predecessor")
	}

	// A patch carrying a region broader than the exact restricted predecessor
	// is rejected before its pending root can publish, and cannot be replayed
	// through a wider view afterwards.
	tooWide := acceptedTransferWrite(t, binding, work, predecessor, target, whole, 1)
	if _, _, transferred := work.Transfer(predecessor, view, []carrier.Patch{tooWide}); transferred {
		t.Fatal("transfer accepted a patch outside its restricted support")
	}
	wholeView, ok := predecessor.Restrict(whole)
	if !ok {
		t.Fatal("whole predecessor view")
	}
	if _, _, transferred := work.Transfer(predecessor, wholeView, []carrier.Patch{tooWide}); transferred {
		t.Fatal("failed restricted patch remained reusable")
	}

	// Equal support is insufficient: both the committed predecessor and the
	// restricted view retain exact immutable root-vector provenance.
	stale := acceptedTransferWrite(t, binding, work, predecessor, target, on, 2)
	successor := writeState(t, work, binding, fixture, predecessor, slot, off, 9)
	successorView, ok := successor.Restrict(on)
	if !ok {
		t.Fatal("successor view")
	}
	if _, _, transferred := work.Transfer(predecessor, successorView, []carrier.Patch{stale}); transferred {
		t.Fatal("view from a different committed predecessor was accepted")
	}

	stale = acceptedTransferWrite(t, binding, work, predecessor, target, on, 3)
	if _, _, transferred := work.Transfer(successor, successorView, []carrier.Patch{stale}); transferred {
		t.Fatal("patch from a different committed predecessor was accepted")
	}

	// The matching view admits the patch, retains only its support, and emits
	// both the typed update and exact structural retraction.
	accepted := acceptedTransferWrite(t, binding, work, predecessor, target, on, 4)
	next, changes, transferred := work.Transfer(predecessor, view, []carrier.Patch{accepted})
	if !transferred || !next.Valid() || !next.Support().Equal(on) || !support.Empty(changes.Added()) || !changes.Removed().Equal(off) || changes.Count() != 1 || changes.FactorCount() != 1 {
		t.Fatalf("restricted transfer shape: ok=%t valid=%t support=%t added-empty=%t removed=%t rows=%d", transferred, next.Valid(), next.Support().Equal(on), support.Empty(changes.Added()), changes.Removed().Equal(off), changes.Count())
	}
	root, ok := next.HandleAt(slot)
	if !ok {
		t.Fatal("transferred root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), on, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("restricted transfer value = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	row, present := changes.At(0)
	if !present || !row.Region().Equal(on) {
		t.Fatal("restricted transfer change region escaped its view")
	}
	factor, present := changes.FactorAt(0)
	if !present || factor.Slot() != slot || !factor.Region().Equal(on) {
		t.Fatal("restricted transfer Factor region escaped its view")
	}
}

func TestTransferEmptyRestrictionRetractsSupportAndCarriesRoots(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, predecessor, slot, composition, _ := bindingState(t, manager, lawInput(true), whole)
	_ = binding
	work := newWork(t, composition)
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	view, ok := predecessor.Restrict(empty)
	if !ok {
		t.Fatal("empty view")
	}
	next, changes, transferred := work.Transfer(predecessor, view, nil)
	if !transferred || !next.Valid() || !support.Empty(next.Support()) || !support.Empty(changes.Added()) || !changes.Removed().Equal(whole) || changes.Count() != 0 || changes.FactorCount() != 0 {
		t.Fatalf("empty transfer shape: ok=%t valid=%t empty=%t added-empty=%t removed=%t rows=%d", transferred, next.Valid(), support.Empty(next.Support()), support.Empty(changes.Added()), changes.Removed().Equal(whole), changes.Count())
	}
	before, beforeOK := predecessor.HandleAt(slot)
	after, afterOK := next.HandleAt(slot)
	if !beforeOK || !afterOK || before != after {
		t.Fatal("empty transfer replaced a carried root")
	}
}

func TestTransferAtomicallyPublishesDistinctFactorsAndKeepsNoOpAllocationFree(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first, firstFixture := newLawBinding(t, manager, true)
	second, secondFixture := newLawBinding(t, manager, true)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	predecessor, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("predecessor")
	}
	work := newWork(t, composition)
	view, ok := predecessor.Restrict(whole)
	if !ok {
		t.Fatal("whole view")
	}
	firstPatch := acceptedTransferWrite(t, first, work, predecessor, firstFixture.target(t, 0, carrier.StrongTarget), whole, 1)
	secondPatch := acceptedTransferWrite(t, second, work, predecessor, secondFixture.target(t, 0, carrier.StrongTarget), whole, 2)
	next, changes, transferred := work.Transfer(predecessor, view, []carrier.Patch{firstPatch, secondPatch})
	if !transferred || !next.Valid() || !next.Support().Equal(whole) || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) || changes.Count() != 2 || changes.FactorCount() != 2 {
		t.Fatalf("distinct-Factor transfer shape: ok=%t valid=%t rows=%d", transferred, next.Valid(), changes.Count())
	}
	for slot, expected := range map[shape.Slot]uint64{0: 1, 1: 2} {
		root, present := next.HandleAt(slot)
		binding := first
		fixture := firstFixture
		if slot == 1 {
			binding = second
			fixture = secondFixture
		}
		if got, exists, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !present || !valid || !exists || got != expected {
			t.Fatalf("slot %d transfer = %d/%t/%t/%t, want %d", slot, got, present, valid, exists, expected)
		}
	}
	for index, slot := range []shape.Slot{0, 1} {
		factor, present := changes.FactorAt(index)
		if !present || factor.Slot() != slot || !factor.Region().Equal(whole) {
			t.Fatal("distinct-Factor transfer regions are not canonical")
		}
	}

	// The common full-support view and no patches are the direct unchanged
	// path. It must retain predecessor identity and need no hot allocations.
	if allocations := testing.AllocsPerRun(100, func() {
		unchanged, set, committed := work.Transfer(next, mustTransferView(t, next, whole), nil)
		if !committed || !set.Empty() || !work.EqualUnder(unchanged, next) {
			t.Fatal("unchanged transfer")
		}
	}); allocations != 0 {
		t.Fatalf("unchanged transfer allocations/run = %g, want 0", allocations)
	}
}

func mustTransferView(t testing.TB, state carrier.State, within support.Mask) carrier.View {
	t.Helper()
	view, ok := state.Restrict(within)
	if !ok {
		t.Fatal("transfer view")
	}
	return view
}
