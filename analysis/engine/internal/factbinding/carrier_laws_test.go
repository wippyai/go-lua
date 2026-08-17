package factbinding

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func scopedWidenScope(t testing.TB, composition *carrier.Composition, fixture testFixture, key uint64) carrier.MergeScope {
	t.Helper()
	selection, ok := composition.SealWidening([]carrier.Target{fixture.target(t, key, carrier.StrongTarget)})
	if !ok {
		t.Fatal("scoped widening selection")
	}
	return selection
}

// selectedPointCell is one authored key/value inside a single Factor.
type selectedPointCell struct {
	target carrier.Target
	value  uint64
}

// selectedPointWrite is the authored surface one Factor contributes. Every cell
// of a Factor is staged together because a contribution admits exactly one
// patch per physical slot.
type selectedPointWrite struct {
	binding *Binding[uint64, uint64]
	cells   []selectedPointCell
}

// closedSelectedPoint builds one closed PointState from independent per-Factor
// authored writes. The recurrence publication boundary consumes nominal point
// roles, so its operands must cross the ordinary contribution closure cut.
func closedSelectedPoint(t testing.TB, work *carrier.Work, plan carrier.ContributionPlan, scope carrier.Scope, authored support.Mask, writes ...selectedPointWrite) carrier.PointState {
	t.Helper()
	base, ok := work.BeginContribution(plan, scope, nil, authored)
	if !ok {
		t.Fatal("begin closed operand")
	}
	patches := make([]carrier.Patch, 0, len(writes))
	for _, write := range writes {
		stage := write.binding.Begin(work, base.State())
		if stage == nil {
			t.Fatal("stage closed operand")
		}
		for _, cell := range write.cells {
			if !stage.Write(cell.target, authored, cell.value) {
				t.Fatal("write closed operand")
			}
		}
		accepted, acceptedOK := stage.Accept(work)
		if !acceptedOK {
			t.Fatal("accept closed operand")
		}
		patches = append(patches, accepted)
	}
	result, ok := work.FinishContribution(base, patches)
	if !ok || !result.Valid() {
		t.Fatal("finish closed operand")
	}
	rule, ok := work.AsRuleContribution(result)
	if !ok {
		t.Fatal("closed operand rule role")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("closed operand point role")
	}
	return point
}

// selectedPointRHS puts one closed PointState into the RHS operand role.
func selectedPointRHS(t testing.TB, work *carrier.Work, point carrier.PointState) carrier.PointRHS {
	t.Helper()
	rhs, ok := work.PointRHSFromPointState(point)
	if !ok {
		t.Fatal("closed operand RHS role")
	}
	return rhs
}

func TestUnselectedWidenReplacesRightRootAndReportsExactDelta(t *testing.T) {
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
	if !ok {
		t.Fatal("off support")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("seal support")
	}
	selected, selectedFixture := newLawBinding(t, manager, true)
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	selectedOnly := scopedWidenScope(t, composition, selectedFixture, 0)
	selectedSlot, passiveSlot := shape.Slot(0), shape.Slot(1)
	left, ok := carrier.NewState(composition, composition.Scope(), on)
	if !ok {
		t.Fatal("left state")
	}
	right, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("right state")
	}
	left = writeState(t, newWork(t, composition), selected, selectedFixture, left, selectedSlot, on, 1)
	left = writeState(t, newWork(t, composition), passive, passiveFixture, left, passiveSlot, on, 9)
	right = writeState(t, newWork(t, composition), selected, selectedFixture, right, selectedSlot, whole, 2)
	right = writeStates(t, newWork(t, composition), passive, passiveFixture, right, passiveSlot, []factWrite{{when: on, value: 4}, {when: off, value: 22}})
	work := newWork(t, composition)
	merged, changes, ok := work.Merge3Under(carrier.Widen, left, right, selectedOnly)
	if !ok {
		t.Fatal("mixed widen")
	}
	if !merged.Support().Equal(whole) || !changes.Added().Equal(off) || !support.Empty(changes.Removed()) || changes.Count() != 2 || changes.FactorCount() != 2 {
		t.Fatalf("mixed Widen shape: support=%t added=%t removed-empty=%t rows=%d", merged.Support().Equal(whole), changes.Added().Equal(off), support.Empty(changes.Removed()), changes.Count())
	}
	for index, input := range []struct {
		binding *Binding[uint64, uint64]
		fixture testFixture
	}{{selected, selectedFixture}, {passive, passiveFixture}} {
		row, present := changes.At(index)
		target := input.fixture.target(t, 0, carrier.StrongTarget)
		slot, slotted := target.Slot()
		units, declared := composition.TargetNotifications(slot, target)
		if !present || !slotted || !declared || len(units) != 1 || !row.Unit().Same(units[0]) || !row.Region().Equal(on) {
			t.Fatalf("mixed Widen row %d is not the exact slot/unit overlap delta", index)
		}
		factor, present := changes.FactorAt(index)
		if !present || factor.Slot() != shape.Slot(index) || !factor.Region().Equal(on) {
			t.Fatalf("mixed Widen Factor row %d is not exact", index)
		}
	}
	if work.LessOrEqUnder(left, merged) || !work.LessOrEqUnder(right, merged) {
		t.Fatal("mixed Widen must order recomputed right below its result, not the replaced left")
	}
	selectedRoot, _ := merged.HandleAt(selectedSlot)
	if got, present, valid := observedExactValue(selected, work, selectedRoot, selectedFixture.unit(t, 0), on, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 2 {
		t.Fatalf("selected Widen did not retain its widened result = %d/%t/%t, want 2/true/true", got, present, valid)
	}
	passiveRoot, ok := merged.HandleAt(passiveSlot)
	if !ok {
		t.Fatal("passive root")
	}
	rightPassiveRoot, _ := right.HandleAt(passiveSlot)
	if passiveRoot != rightPassiveRoot {
		t.Fatal("unselected Widen did not install exact right root")
	}
	if got, present, valid := observedExactValue(passive, work, passiveRoot, passiveFixture.unit(t, 0), on, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("overlap replacement = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(passive, work, passiveRoot, passiveFixture.unit(t, 0), off, func(guard.Atom) bool { return false }); !valid || !present || got != 22 {
		t.Fatalf("right-only root = %d/%t/%t, want 22/true/true", got, present, valid)
	}
}

func TestUnselectedEqualSupportWidenRetainsRightRootForLaterGrowth(t *testing.T) {
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
	selected, selectedFixture := newLawBinding(t, manager, true)
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	selection := scopedWidenScope(t, composition, selectedFixture, 0)
	passiveSlot := shape.Slot(1)
	base, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("base")
	}
	leftFull := writeStates(t, newWork(t, composition), passive, passiveFixture, base, passiveSlot, []factWrite{{when: on, value: 3}, {when: off, value: 11}})
	rightFull := writeStates(t, newWork(t, composition), passive, passiveFixture, base, passiveSlot, []factWrite{{when: on, value: 3}, {when: off, value: 22}})
	leftView, ok := leftFull.Restrict(on)
	if !ok {
		t.Fatal("left view")
	}
	left, _, ok := newWork(t, composition).Transfer(leftFull, leftView, nil)
	if !ok {
		t.Fatal("left restricted")
	}
	rightView, ok := rightFull.Restrict(on)
	if !ok {
		t.Fatal("right view")
	}
	right, _, ok := newWork(t, composition).Transfer(rightFull, rightView, nil)
	if !ok {
		t.Fatal("right restricted")
	}
	leftRoot, _ := left.HandleAt(passiveSlot)
	rightRoot, _ := right.HandleAt(passiveSlot)
	if leftRoot == rightRoot {
		t.Fatal("distinct hidden roots")
	}
	work := newWork(t, composition)
	next, changes, ok := work.Merge3Under(carrier.Widen, left, right, selection)
	if !ok || !next.Support().Equal(on) || !changes.Empty() || changes.FactorCount() != 0 {
		t.Fatalf("equal-support Widen: ok=%t support=%t changes-empty=%t", ok, next.Support().Equal(on), changes.Empty())
	}
	nextRoot, _ := next.HandleAt(passiveSlot)
	if nextRoot != rightRoot {
		t.Fatal("equal unselected Widen kept the old root")
	}

	// The second Widen is a pure carrier growth.  Its right root is already
	// installed, so no old off-support meaning may become reachable.
	grown, growth, ok := work.Merge3Under(carrier.Widen, next, rightFull, selection)
	if !ok || !grown.Support().Equal(whole) || !growth.Added().Equal(off) || !support.Empty(growth.Removed()) || growth.Count() != 0 || growth.FactorCount() != 0 || !work.EqualUnder(grown, rightFull) {
		t.Fatalf("later growth: ok=%t support=%t added=%t removed-empty=%t rows=%d equal=%t", ok, grown.Support().Equal(whole), growth.Added().Equal(off), support.Empty(growth.Removed()), growth.Count(), work.EqualUnder(grown, rightFull))
	}
	grownRoot, _ := grown.HandleAt(passiveSlot)
	if got, present, valid := observedExactValue(passive, work, grownRoot, passiveFixture.unit(t, 0), off, func(guard.Atom) bool { return false }); !valid || !present || got != 22 {
		t.Fatalf("later growth revealed old root = %d/%t/%t, want 22/true/true", got, present, valid)
	}
}

func TestScopedWidenUsesTargetScopeAndInstallsExactRightElsewhere(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	config := lawInput(true)
	config.Widen = func(left, right uint64) uint64 {
		if left == right {
			return left
		}
		return 99
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, config, whole)
	target := fixture.target(t, 0, carrier.StrongTarget)
	selection, ok := composition.SealWidening([]carrier.Target{target})
	if !ok {
		t.Fatal("scoped widening seal")
	}
	writeKey := func(state carrier.State, key, value uint64) carrier.State {
		work := newWork(t, composition)
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(fixture.target(t, key, carrier.StrongTarget), whole, value) {
			t.Fatal("write")
		}
		accepted, ok := patch.Accept(work)
		if !ok {
			t.Fatal("accept")
		}
		return commit(t, work, state, accepted)
	}
	left := writeKey(writeKey(base, 0, 1), 1, 10)
	right := writeKey(writeKey(base, 0, 2), 1, 20)
	merged, _, ok := newWork(t, composition).Merge3Under(carrier.Widen, left, right, selection)
	if !ok {
		t.Fatal("scoped widening")
	}
	root, ok := merged.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	if value, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || value != 99 {
		t.Fatalf("selected key = %d/%t/%t, want 99/true/true", value, present, valid)
	}
	if value, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || value != 20 {
		t.Fatalf("unselected key = %d/%t/%t, want exact right 20/true/true", value, present, valid)
	}
}

func TestMergeSelectedPointStateUsesExactRightOutsideSelectedKeys(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	selectedInput := lawInput(true)
	selectedInput.Widen = func(left, right uint64) uint64 {
		if left == right {
			return left
		}
		return 99
	}
	selectedFixture := newTestFixture(selectedInput.KeyEnd)
	selectedInput.declare = selectedFixture.declareAllExact
	selected, ok := bindTest(selectedInput, manager)
	if !ok {
		t.Fatal("selected binding")
	}
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	selection := scopedWidenScope(t, composition, selectedFixture, 0)
	plan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("all-writes plan")
	}
	selectedSlot, passiveSlot := shape.Slot(0), shape.Slot(1)
	work := newWork(t, composition)
	selectedKey0 := selectedFixture.target(t, 0, carrier.StrongTarget)
	selectedKey1 := selectedFixture.target(t, 1, carrier.StrongTarget)
	passiveKey0 := passiveFixture.target(t, 0, carrier.StrongTarget)

	// Selected key 0 takes the Widen of current and selectedRight. Every other
	// authored coordinate - the unselected key 1 and the whole passive Factor -
	// must install exactRight, so each is deliberately lower in exactRight than
	// in selectedRight. exactRight stays below selectedRight everywhere, which
	// the closed recurrence requires before it publishes.
	current := closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 1}, {selectedKey1, 10}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 9}}},
	)
	selectedRight := selectedPointRHS(t, work, closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 2}, {selectedKey1, 30}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 40}}},
	))
	exactRight := selectedPointRHS(t, work, closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 2}, {selectedKey1, 20}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 4}}},
	))
	next, changes, ok := work.MergeSelectedPointState(carrier.Widen, current, selectedRight, exactRight, selection)
	if !ok || !next.Support().Equal(whole) || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) {
		t.Fatalf("selected merge shape: ok=%t support=%t added-empty=%t removed-empty=%t", ok, next.Support().Equal(whole), support.Empty(changes.Added()), support.Empty(changes.Removed()))
	}
	selectedRoot, _ := next.HandleAt(selectedSlot)
	for _, want := range []struct {
		key, value uint64
	}{{0, 99}, {1, 20}} {
		got, present, valid := observedExactValue(selected, work, selectedRoot, selectedFixture.unit(t, want.key), whole, func(guard.Atom) bool { return false })
		if !valid || !present || got != want.value {
			t.Fatalf("selected key %d = %d/%t/%t, want %d/true/true", want.key, got, present, valid, want.value)
		}
	}
	passiveRoot, _ := next.HandleAt(passiveSlot)
	if got, present, valid := observedExactValue(passive, work, passiveRoot, passiveFixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("stale unselected value = %d/%t/%t, want 4/true/true", got, present, valid)
	}
}

func TestMergeSelectedPointStateWidenRequiresSelectedAndExactSupportEquality(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("seal support")
	}
	binding, fixture := newLawBinding(t, manager, true)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("composition")
	}
	selection := scopedWidenScope(t, composition, fixture, 0)
	currentState, ok := carrier.NewState(composition, composition.Scope(), on)
	if !ok {
		t.Fatal("current")
	}
	exactState, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("exact")
	}
	work := newWork(t, composition)
	current, ok := work.EmptyPointState(currentState)
	if !ok {
		t.Fatal("current point")
	}
	currentRHS, ok := work.PointRHSFromPointState(current)
	if !ok {
		t.Fatal("current RHS")
	}
	exactPoint, ok := work.EmptyPointState(exactState)
	if !ok {
		t.Fatal("exact point")
	}
	exact, ok := work.PointRHSFromPointState(exactPoint)
	if !ok {
		t.Fatal("exact RHS")
	}
	if next, _, accepted := work.MergeSelectedPointState(carrier.Widen, current, currentRHS, exact, selection); accepted || next.Valid() {
		t.Fatal("widen accepted selected/exact support mismatch")
	}
}

func TestMergeSelectedPointStateNarrowUsesFullExactRight(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	selectedInput := lawInput(true)
	selectedInput.Narrow = func(left, right uint64) uint64 {
		if left < right {
			return left
		}
		return right
	}
	selectedInput.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	selectedFixture := newTestFixture(selectedInput.KeyEnd)
	selectedInput.declare = selectedFixture.declareAllExact
	selected, ok := bindTest(selectedInput, manager)
	if !ok {
		t.Fatal("selected binding")
	}
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	selection, ok := composition.SealNarrowing([]carrier.Target{selectedFixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow selection")
	}
	plan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("all-writes plan")
	}
	selectedSlot, passiveSlot := shape.Slot(0), shape.Slot(1)
	work := newWork(t, composition)
	selectedKey0 := selectedFixture.target(t, 0, carrier.StrongTarget)
	selectedKey1 := selectedFixture.target(t, 1, carrier.StrongTarget)
	passiveKey0 := passiveFixture.target(t, 0, carrier.StrongTarget)

	// Narrow takes one exact desired RHS, so the selected operand is that same
	// closed surface. Selected key 0 descends to the narrow result while the
	// unselected key 1 and the whole passive Factor install exactRight, none of
	// which may retain the higher current value.
	current := closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 3}, {selectedKey1, 9}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 9}}},
	)
	exact := selectedPointRHS(t, work, closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 2}, {selectedKey1, 4}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 4}}},
	))
	next, _, ok := work.MergeSelectedPointState(carrier.Narrow, current, exact, exact, selection)
	if !ok || !next.Support().Equal(whole) {
		t.Fatalf("narrow selected merge: ok=%t support=%t", ok, next.Support().Equal(whole))
	}
	selectedRoot, _ := next.HandleAt(selectedSlot)
	if got, present, valid := observedExactValue(selected, work, selectedRoot, selectedFixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 2 {
		t.Fatalf("selected narrow = %d/%t/%t, want 2/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(selected, work, selectedRoot, selectedFixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("unselected narrow key = %d/%t/%t, want exact right 4/true/true", got, present, valid)
	}
	passiveRoot, _ := next.HandleAt(passiveSlot)
	if got, present, valid := observedExactValue(passive, work, passiveRoot, passiveFixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("passive narrow exact right = %d/%t/%t, want 4/true/true", got, present, valid)
	}
}

// TestMergeSelectedPointStateNarrowRejectsUnselectedExactRightGrowth proves
// that Narrow is a whole-carrier descent, not merely a descent in the selected
// Factor. The selected coordinate is a valid narrowing candidate, but the
// exact-right surface grows the unselected Factor; publishing that mixed result
// would make the transition globally incomparable.
func TestMergeSelectedPointStateNarrowRejectsUnselectedExactRightGrowth(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	selectedInput := lawInput(true)
	selectedInput.Narrow = func(left, right uint64) uint64 {
		if left < right {
			return left
		}
		return right
	}
	selectedInput.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	selectedFixture := newTestFixture(selectedInput.KeyEnd)
	selectedInput.declare = selectedFixture.declareAllExact
	selected, ok := bindTest(selectedInput, manager)
	if !ok {
		t.Fatal("selected binding")
	}
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	selection, ok := composition.SealNarrowing([]carrier.Target{selectedFixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow selection")
	}
	plan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("all-writes plan")
	}
	work := newWork(t, composition)
	selectedKey0 := selectedFixture.target(t, 0, carrier.StrongTarget)
	passiveKey0 := passiveFixture.target(t, 0, carrier.StrongTarget)

	current := closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 3}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 5}}},
	)
	exactPoint := closedSelectedPoint(t, work, plan, composition.Scope(), whole,
		selectedPointWrite{selected, []selectedPointCell{{selectedKey0, 1}}},
		selectedPointWrite{passive, []selectedPointCell{{passiveKey0, 6}}},
	)
	exact := selectedPointRHS(t, work, exactPoint)
	currentRHS := selectedPointRHS(t, work, current)
	if work.LessOrEqPointRHS(exact, currentRHS) {
		t.Fatal("exact right unexpectedly below current despite unselected growth")
	}
	if next, _, accepted := work.MergeSelectedPointState(carrier.Narrow, current, exact, exact, selection); accepted || next.Valid() {
		t.Fatal("narrow accepted exact-right growth in unselected Factor")
	}
}

func TestSlotWorkRejectsUnscopedWiden(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	left := writeState(t, newWork(t, composition), binding, fixture, base, slot, whole, 1)
	right := writeState(t, newWork(t, composition), binding, fixture, base, slot, whole, 2)
	leftRoot, ok := left.HandleAt(slot)
	if !ok {
		t.Fatal("left root")
	}
	rightRoot, ok := right.HandleAt(slot)
	if !ok {
		t.Fatal("right root")
	}
	split, ok := support.Three(whole, whole)
	if !ok {
		t.Fatal("split")
	}
	delta := support.New(manager)
	if delta == nil {
		t.Fatal("delta")
	}
	defer delta.Discard()
	slotWork, ok := newWork(t, composition).SlotWork(slot)
	if !ok {
		t.Fatal("slot work")
	}
	if _, ok := slotWork.Merge3Under(carrier.Widen, true, 0, leftRoot, rightRoot, split, delta); ok {
		t.Fatal("typed SlotWork accepted an unscoped Widen")
	}
}

func TestWideningCanonicalizesPermutedStagedTargets(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	config := lawInput(true)
	config.Widen = func(left, right uint64) uint64 {
		if left == right {
			return left
		}
		return 99
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, config, whole)
	zero := fixture.target(t, 0, carrier.StrongTarget)
	one := fixture.target(t, 1, carrier.StrongTarget)
	selection, ok := composition.SealWidening([]carrier.Target{one, zero, one})
	if !ok {
		t.Fatal("permuted staged scope")
	}
	writeKey := func(state carrier.State, key, value uint64) carrier.State {
		work := newWork(t, composition)
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(fixture.target(t, key, carrier.StrongTarget), whole, value) {
			t.Fatal("write")
		}
		accepted, ok := patch.Accept(work)
		if !ok {
			t.Fatal("accept")
		}
		return commit(t, work, state, accepted)
	}
	left := writeKey(writeKey(base, 0, 1), 1, 10)
	right := writeKey(writeKey(base, 0, 2), 1, 20)
	merged, _, ok := newWork(t, composition).Merge3Under(carrier.Widen, left, right, selection)
	if !ok {
		t.Fatal("permuted scoped widening")
	}
	root, _ := merged.HandleAt(slot)
	for _, key := range []uint64{0, 1} {
		if value, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, key), whole, func(guard.Atom) bool { return false }); !valid || !present || value != 99 {
			t.Fatalf("selected key %d = %d/%t/%t, want 99/true/true", key, value, present, valid)
		}
	}
}

func TestWideningScopeRejectsAfterWorkCreation(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, _, _, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	target := fixture.target(t, 0, carrier.StrongTarget)
	if _, ok := composition.NewWork(); !ok {
		t.Fatal("work")
	}
	if _, ok := composition.SealWidening([]carrier.Target{target}); ok {
		t.Fatal("composition admitted a scope after work creation")
	}
	if _, ok := binding.PrepareWidening([]carrier.Target{target}); ok {
		t.Fatal("Binding admitted a direct scope after work creation")
	}
}

func TestCarrierRejectsForeignStaleAndSupportDistinctPatchPredecessors(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	binding, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	initialWork := newWork(t, composition)
	nextState := writeState(t, initialWork, binding, fixture, state, slot, whole, 1)
	staged := binding.Begin(initialWork, state)
	if staged == nil || !staged.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 2) {
		t.Fatal("stale stage construction")
	}
	staleWork := newWork(t, composition)
	stale, ok := staged.Accept(staleWork)
	if !ok {
		t.Fatal("stale stage accept")
	}
	if _, _, ok := staleWork.Commit(nextState, []carrier.Patch{stale}); ok {
		t.Fatal("patch from earlier root vector committed to successor")
	}

	foreign, _, _, _, _ := bindingState(t, manager, lawInput(true), whole)
	if foreign.Begin(staleWork, nextState) != nil {
		t.Fatal("foreign binding opened another composition's predecessor")
	}

	first, _ := newLawBinding(t, manager, true)
	second, _ := newLawBinding(t, manager, false)
	multi, ok := attachTestComposition(t, []carrier.FactorOperation{first, second})
	if !ok {
		t.Fatal("wrong-slot composition")
	}
	multiState, ok := carrier.NewState(multi, multi.Scope(), whole)
	if !ok {
		t.Fatal("wrong-slot state")
	}
	multiWork := newWork(t, multi)
	if first.Begin(multiWork, multiState) == nil || second.Begin(multiWork, multiState) == nil {
		t.Fatal("binding opened a stage through another Factor slot")
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
	supportBinding, onState, _, composition, supportFixture := bindingState(t, manager, lawInput(true), on)
	offState, ok := carrier.NewState(composition, composition.Scope(), off)
	if !ok {
		t.Fatal("off state")
	}
	supportWork := newWork(t, composition)
	stagedSupport := supportBinding.Begin(supportWork, onState)
	if stagedSupport == nil || !stagedSupport.Write(supportFixture.target(t, 0, carrier.StrongTarget), on, 0) {
		t.Fatal("support-distinct no-op stage")
	}
	supportPatch, ok := stagedSupport.Accept(supportWork)
	if !ok {
		t.Fatal("support-distinct no-op accept")
	}
	if _, _, ok := supportWork.Commit(offState, []carrier.Patch{supportPatch}); ok {
		t.Fatal("support-distinct predecessor accepted a patch")
	}
}

func TestBindingPatchCannotWriteOutsideItsPredecessorSupport(t *testing.T) {
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
	binding, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), on)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("stage")
	}
	if patch.Write(fixture.target(t, 0, carrier.StrongTarget), off, 1) {
		t.Fatal("strong write escaped predecessor support")
	}
	if patch.Write(fixture.target(t, 0, carrier.WeakTarget), off, 1) {
		t.Fatal("weak write escaped predecessor support")
	}
	if !patch.Write(fixture.target(t, 0, carrier.StrongTarget), on, 1) {
		t.Fatal("write inside predecessor support was rejected")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accepted contained write")
	}
	next := commit(t, work, state, candidate)
	root, ok := next.HandleAt(slot)
	if !ok {
		t.Fatal("published root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), on, func(guard.Atom) bool { return true }); !valid || !present || got != 1 {
		t.Fatalf("contained write = %d/%t/%t, want 1/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), on, func(guard.Atom) bool { return false }); valid || present || got != 0 {
		t.Fatalf("outside predecessor support changed = %d/%t/%t, want 0/false/false", got, present, valid)
	}
}

func TestBindingCarrierHotStateOperationsAllocateNothing(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	binding, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	staged := binding.Begin(work, state)
	if staged == nil || !staged.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 0) {
		t.Fatal("no-op stage")
	}
	noChange, ok := staged.Accept(work)
	if !ok {
		t.Fatal("no-op accept")
	}
	noChanges := []carrier.Patch{noChange}
	all := composition.AllMergeScope()
	assertNoAllocs(t, "valid", func() { _ = state.Valid() })
	assertNoAllocs(t, "handle", func() { _, _ = state.HandleAt(slot) })
	assertNoAllocs(t, "equal-self", func() { _ = work.EqualUnder(state, state) })
	assertNoAllocs(t, "less-self", func() { _ = work.LessOrEqUnder(state, state) })
	assertNoAllocs(t, "commit-empty", func() { _, _, _ = work.Commit(state, nil) })
	assertNoAllocs(t, "commit-no-op", func() { _, _, _ = work.Commit(state, noChanges) })
	assertNoAllocs(t, "join-self", func() { _, _, _ = work.Merge3Under(carrier.Join, state, state, all) })
}

func TestWorkRejectsForeignCompositionAndSeparateWorksReadSafely(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	otherBinding, otherState, _, otherComposition, _ := bindingState(t, manager, lawInput(true), whole)
	_ = otherBinding
	work := newWork(t, composition)
	if work.EqualUnder(state, otherState) || work.LessOrEqUnder(state, otherState) {
		t.Fatal("work accepted a foreign composition")
	}
	if _, _, ok := work.Merge3Under(carrier.Join, state, otherState, composition.AllMergeScope()); ok {
		t.Fatal("work merged a foreign composition")
	}
	if foreign := newWork(t, otherComposition); foreign.EqualUnder(state, otherState) {
		t.Fatal("foreign work compared another composition")
	}
	left := writeState(t, newWork(t, composition), first, fixture, state, slot, whole, 1)
	right := writeState(t, newWork(t, composition), first, fixture, state, slot, whole, 2)
	firstWork, secondWork := newWork(t, composition), newWork(t, composition)
	var group sync.WaitGroup
	for _, reader := range []*carrier.Work{firstWork, secondWork} {
		group.Add(1)
		go func(reader *carrier.Work) {
			defer group.Done()
			for count := 0; count < 1_000; count++ {
				if reader.EqualUnder(left, right) || !reader.LessOrEqUnder(left, right) {
					t.Errorf("inconsistent immutable read")
					return
				}
			}
		}(reader)
	}
	group.Wait()
}

func TestSeparateWorksCommitDistinctRootsWithoutCollision(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	leftBinding, leftState, leftSlot, leftComposition, leftFixture := bindingState(t, manager, lawInput(true), whole)
	rightBinding, rightState, rightSlot, rightComposition, rightFixture := bindingState(t, manager, lawInput(true), whole)
	stage := func(binding *Binding[uint64, uint64], fixture testFixture, predecessor carrier.State, work *carrier.Work, value uint64) carrier.Patch {
		patch := binding.Begin(work, predecessor)
		if patch == nil || !patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, value) {
			t.Fatal("stage")
		}
		candidate, accepted := patch.Accept(work)
		if !accepted {
			t.Fatal("accept")
		}
		return candidate
	}
	type outcome struct {
		state carrier.State
		ok    bool
	}
	for round := uint64(0); round < 32; round++ {
		leftValue, rightValue := round*2+1, round*2+2
		leftWork, rightWork := newWork(t, leftComposition), newWork(t, rightComposition)
		leftPatch := stage(leftBinding, leftFixture, leftState, leftWork, leftValue)
		rightPatch := stage(rightBinding, rightFixture, rightState, rightWork, rightValue)
		start := make(chan struct{})
		results := [2]outcome{}
		var group sync.WaitGroup
		for index, input := range []struct {
			state carrier.State
			work  *carrier.Work
			patch carrier.Patch
		}{
			{state: leftState, work: leftWork, patch: leftPatch},
			{state: rightState, work: rightWork, patch: rightPatch},
		} {
			group.Add(1)
			go func(index int, input struct {
				state carrier.State
				work  *carrier.Work
				patch carrier.Patch
			}) {
				defer group.Done()
				<-start
				next, _, committed := input.work.Commit(input.state, []carrier.Patch{input.patch})
				results[index] = outcome{state: next, ok: committed}
			}(index, input)
		}
		close(start)
		group.Wait()
		if !results[0].ok || !results[1].ok {
			t.Fatal("concurrent commits")
		}
		leftRoot, leftOK := results[0].state.HandleAt(leftSlot)
		rightRoot, rightOK := results[1].state.HandleAt(rightSlot)
		if !leftOK || !rightOK {
			t.Fatal("concurrent roots unavailable")
		}
		for index, expected := range []uint64{leftValue, rightValue} {
			root := [2]carrier.RootHandle{leftRoot, rightRoot}[index]
			reader := [2]*carrier.Work{leftWork, rightWork}[index]
			binding := [2]*Binding[uint64, uint64]{leftBinding, rightBinding}[index]
			fixture := [2]testFixture{leftFixture, rightFixture}[index]
			if got, present, valid := observedExactValue(binding, reader, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != expected {
				t.Fatalf("concurrent output %d = %d/%t/%t, want %d", index, got, present, valid, expected)
			}
		}
	}
}

func TestWorkUsesHeterogeneousTypedSlotsWithoutPayloadErasure(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var byteTarget carrier.Target
	byteBinding, ok := bindTest(testAlgebraInput[uint64, uint8]{
		KeyEnd:      1,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint8) bool { return true },
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        func(left, right uint8) uint8 { return left | right },
		Widen:       func(left, right uint8) uint8 { return left | right },
		LessOrEq:    func(left, right uint8) bool { return left&right == left },
		declare: func(binding *Binding[uint64, uint8]) bool {
			unit, ok := binding.DeclareExact(0)
			if !ok {
				return false
			}
			byteTarget, ok = binding.DeclareStrong(unit)
			return ok
		},
	}, manager)
	if !ok {
		t.Fatal("byte binding")
	}
	wordBinding, wordFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{wordBinding, byteBinding})
	if !ok {
		t.Fatal("heterogeneous composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work := newWork(t, composition)
	bytePatch := byteBinding.Begin(work, state)
	if bytePatch == nil || !bytePatch.Write(byteTarget, whole, 1) {
		t.Fatal("byte write")
	}
	byteCarrierPatch, ok := bytePatch.Accept(work)
	if !ok {
		t.Fatal("byte accept")
	}
	byteState := commit(t, work, state, byteCarrierPatch)
	wordPatch := wordBinding.Begin(work, state)
	if wordPatch == nil || !wordPatch.Write(wordFixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("word write")
	}
	wordCarrierPatch, ok := wordPatch.Accept(work)
	if !ok {
		t.Fatal("word accept")
	}
	wordState := commit(t, work, state, wordCarrierPatch)
	if work.EqualUnder(state, byteState) || work.EqualUnder(state, wordState) {
		t.Fatal("typed slot dispatch lost a changed factor")
	}
}

func TestWorkWarmedNonidenticalRelationsAllocateNothing(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	left := writeState(t, newWork(t, composition), binding, fixture, state, slot, whole, 1)
	right := writeState(t, newWork(t, composition), binding, fixture, state, slot, whole, 2)
	work := newWork(t, composition)
	if work.EqualUnder(left, right) || !work.LessOrEqUnder(left, right) {
		t.Fatal("relation setup")
	}
	assertNoAllocs(t, "equal-nonidentical", func() { _ = work.EqualUnder(left, right) })
	assertNoAllocs(t, "less-nonidentical", func() { _ = work.LessOrEqUnder(left, right) })
}

func TestSelectionAdmitsOnlyDeclaredRecurrenceAndNarrowDoesNotResurrectLeft(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	rightSupport, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("right support")
	}
	selectedInput := lawInput(true)
	selectedInput.Narrow = func(left, right uint64) uint64 {
		if left < right {
			return left
		}
		return right
	}
	selectedInput.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	selectedFixture := newTestFixture(selectedInput.KeyEnd)
	selectedInput.declare = selectedFixture.declareAllExact
	selected, ok := bindTest(selectedInput, manager)
	if !ok {
		t.Fatal("selected binding")
	}
	passive, passiveFixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{selected, passive})
	if !ok {
		t.Fatal("composition")
	}
	passiveSlot := shape.Slot(1)
	if _, ok := composition.SealWidening([]carrier.Target{passiveFixture.target(t, 0, carrier.StrongTarget)}); ok {
		t.Fatal("selected widen without rank")
	}
	selection, ok := composition.SealNarrowing([]carrier.Target{selectedFixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow selection")
	}
	left, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("left")
	}
	right, ok := carrier.NewState(composition, composition.Scope(), rightSupport)
	if !ok {
		t.Fatal("right")
	}
	left = writeStates(t, newWork(t, composition), passive, passiveFixture, left, passiveSlot, []factWrite{{when: whole, value: 3}})
	right = writeStates(t, newWork(t, composition), passive, passiveFixture, right, passiveSlot, []factWrite{{when: rightSupport, value: 3}})
	rightRoot, _ := right.HandleAt(passiveSlot)
	next, _, ok := newWork(t, composition).Merge3Under(carrier.Narrow, left, right, selection)
	if !ok {
		t.Fatal("narrow")
	}
	nextRoot, _ := next.HandleAt(passiveSlot)
	if nextRoot != rightRoot {
		t.Fatal("unselected narrow rebuilt a union instead of retaining right root")
	}
}

// BenchmarkBindingWorkChangedWiden reports the real cost of producing a new
// persistent result. It deliberately has no zero-allocation assertion: every
// successful changed merge publishes a new typed root and may add immutable
// terminal/FDD structure.
func BenchmarkBindingWorkChangedWiden(b *testing.B) {
	manager, err := guard.New(nil)
	if err != nil {
		b.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		b.Fatal("support")
	}
	config := lawInput(true)
	config.Widen = func(left, right uint64) uint64 {
		if left == right {
			return left
		}
		return 7
	}
	binding, state, slot, composition, fixture := bindingState(b, manager, config, whole)
	selection := scopedWidenScope(b, composition, fixture, 0)
	left := writeState(b, newWork(b, composition), binding, fixture, state, slot, whole, 2)
	right := writeState(b, newWork(b, composition), binding, fixture, state, slot, whole, 3)
	work := newWork(b, composition)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, _, ok := work.Merge3Under(carrier.Widen, left, right, selection); !ok {
			b.Fatal("widen")
		}
	}
}

// BenchmarkBindingBeginAdmissionFactorCount makes the relevant scaling claim
// visible: Begin's carrier admission checks only the supplied Work, exact
// composition, and target slot. The candidate stage itself has fixed costs,
// but neither allocations nor elapsed time should grow with unrelated Factors.
func BenchmarkBindingBeginAdmissionFactorCount(b *testing.B) {
	b.Run("factors=1", func(b *testing.B) { benchmarkBindingBeginAdmission(b, 1) })
	b.Run("factors=64", func(b *testing.B) { benchmarkBindingBeginAdmission(b, 64) })
}

func benchmarkBindingBeginAdmission(b *testing.B, factorCount int) {
	b.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		b.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		b.Fatal("support")
	}
	operations := make([]carrier.FactorOperation, factorCount)
	var binding *Binding[uint64, uint64]
	for index := range operations {
		candidate, _ := newLawBinding(b, manager, false)
		operations[index] = candidate
		if index == 0 {
			binding = candidate
		}
	}
	composition, ok := attachTestComposition(b, operations)
	if !ok {
		b.Fatal("composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		b.Fatal("state")
	}
	work := newWork(b, composition)
	warm := binding.Begin(work, state)
	if warm == nil || !warm.Discard() {
		b.Fatal("warm begin")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Discard() {
			b.Fatal("begin")
		}
	}
}

func assertNoAllocs(t testing.TB, name string, operation func()) {
	t.Helper()
	operation()
	if allocations := testing.AllocsPerRun(100, operation); allocations != 0 {
		t.Fatalf("%s allocations/run = %g, want 0", name, allocations)
	}
}

func lawInput(recurrence bool) testAlgebraInput[uint64, uint64] {
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      4,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
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
		LessOrEq: func(left, right uint64) bool { return left <= right },
	}
	if recurrence {
		config.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	}
	return config
}

func newLawBinding(t testing.TB, manager *guard.Manager, recurrence bool) (*Binding[uint64, uint64], testFixture) {
	t.Helper()
	fixture := newTestFixture(4)
	config := lawInput(recurrence)
	config.declare = fixture.declareAllExact
	binding, ok := bindTest(config, manager)
	if !ok {
		t.Fatal("law binding")
	}
	return binding, fixture
}

func writeState(t testing.TB, work *carrier.Work, binding *Binding[uint64, uint64], fixture testFixture, state carrier.State, slot shape.Slot, when support.Mask, value uint64) carrier.State {
	return writeStates(t, work, binding, fixture, state, slot, []factWrite{{when: when, value: value}})
}

type factWrite struct {
	when  support.Mask
	value uint64
}

func writeStates(t testing.TB, work *carrier.Work, binding *Binding[uint64, uint64], fixture testFixture, state carrier.State, slot shape.Slot, writes []factWrite) carrier.State {
	t.Helper()
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("write stage")
	}
	for _, write := range writes {
		if !patch.Write(fixture.target(t, 0, carrier.StrongTarget), write.when, write.value) {
			t.Fatal("write stage")
		}
	}
	next, ok := patch.Accept(work)
	if !ok {
		t.Fatal("write accept")
	}
	return commit(t, work, state, next)
}
