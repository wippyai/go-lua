package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// selfScopeDischarge seals the relation a recurrence head widens its support
// axis with: its own coordinate scope, with exactly the named coordinates
// existentially discharged and every other coordinate retained.
func selfScopeDischarge(t testing.TB, composition *carrier.Composition, scope carrier.Scope, retained, discharged []guard.Atom) carrier.ReindexPlan {
	t.Helper()
	builder, ok := composition.NewReindex(scope, scope)
	if !ok {
		t.Fatal("self-scope discharge builder")
	}
	for _, atom := range retained {
		if !builder.Identity(atom) {
			t.Fatalf("retain atom %d", atom)
		}
	}
	for _, atom := range discharged {
		if !builder.Forget(atom) {
			t.Fatalf("discharge atom %d", atom)
		}
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("self-scope discharge plan")
	}
	return plan
}

// TestSupportAxisWideningJoinsDischargedCellsAndRetainsTheScope is the
// soundness law for the support-axis counterpart of value widening. A head
// discharges a cycle-local coordinate by transporting its state through a
// self-relation that forgets exactly that coordinate. Three things have to
// hold at once, or the operator is not a widening: the discharged plane is
// above the exact one at every cell of the original support, the retained
// coordinate still separates the cells it separated before, and the head keeps
// the exact coordinate interface every downstream transport was sealed
// against.
func TestSupportAxisWideningJoinsDischargedCellsAndRetainsTheScope(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	cycleOn, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("cycle-local coordinate")
	}
	cycleOff, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("cycle-local complement")
	}
	outerOn, ok := regions.Literal(2, true)
	if !ok {
		t.Fatal("outer coordinate")
	}
	outerOff, ok := regions.Literal(2, false)
	if !ok || !regions.Seal() {
		t.Fatal("outer complement")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	plan := selfScopeDischarge(t, composition, base.Scope(), []guard.Atom{2}, []guard.Atom{1})

	// One iteration-local distinction (atom 1) crossed with one distinction the
	// cycle does not own (atom 2).
	cells := []struct {
		when  support.Mask
		value uint64
	}{
		{when: mustAnd(t, cycleOff, outerOff), value: 1},
		{when: mustAnd(t, cycleOn, outerOff), value: 5},
		{when: mustAnd(t, cycleOff, outerOn), value: 2},
		{when: mustAnd(t, cycleOn, outerOn), value: 9},
	}
	writes := make([]factWrite, 0, len(cells))
	for _, cell := range cells {
		writes = append(writes, factWrite{when: cell.when, value: cell.value})
	}
	exact := writeStates(t, newWork(t, composition), binding, fixture, base, slot, writes)
	discharged, ok := newWork(t, composition).Reindex(exact, plan)
	if !ok {
		t.Fatal("support-axis widening did not transport the head")
	}
	if !discharged.Scope().Same(exact.Scope()) {
		t.Fatal("support-axis widening replaced the head's coordinate interface")
	}
	if !discharged.Support().Equal(whole) {
		t.Fatal("support-axis widening did not retain the head's feasibility region")
	}
	exactRoot, exactRootOK := exact.HandleAt(slot)
	dischargedRoot, dischargedRootOK := discharged.HandleAt(slot)
	if !exactRootOK || !dischargedRootOK {
		t.Fatal("head roots")
	}
	unit := fixture.unit(t, 0)
	// The discharged plane is above the exact one at every valuation of the
	// original support: this is the widening half of the law.
	for _, valuation := range []map[guard.Atom]bool{
		{1: false, 2: false}, {1: true, 2: false}, {1: false, 2: true}, {1: true, 2: true},
	} {
		at := func(atom guard.Atom) bool { return valuation[atom] }
		before, beforePresent, beforeValid := observedExactValue(binding, newWork(t, composition), exactRoot, unit, whole, at)
		after, afterPresent, afterValid := observedExactValue(binding, newWork(t, composition), dischargedRoot, unit, whole, at)
		if !beforeValid || !afterValid || !beforePresent || !afterPresent {
			t.Fatalf("valuation %v observation = %t/%t before, %t/%t after", valuation, beforeValid, beforePresent, afterValid, afterPresent)
		}
		if after < before {
			t.Fatalf("support-axis widening lowered the head at valuation %v: %d -> %d", valuation, before, after)
		}
	}
	// The discharged coordinate no longer separates cells; the retained one
	// still does. Losing the second would make this a collapse, not a widening
	// of one declared axis.
	lowOuterOff, _, lowOK := observedExactValue(binding, newWork(t, composition), dischargedRoot, unit, whole, func(atom guard.Atom) bool { return atom == 1 })
	highOuterOff, _, highOK := observedExactValue(binding, newWork(t, composition), dischargedRoot, unit, whole, func(guard.Atom) bool { return false })
	if !lowOK || !highOK || lowOuterOff != highOuterOff || lowOuterOff != 5 {
		t.Fatalf("discharged coordinate still separates the head: %d vs %d, want the join 5", highOuterOff, lowOuterOff)
	}
	outerTrue, _, outerTrueOK := observedExactValue(binding, newWork(t, composition), dischargedRoot, unit, whole, func(atom guard.Atom) bool { return atom == 2 })
	if !outerTrueOK || outerTrue != 9 {
		t.Fatalf("retained coordinate lost its distinction: outer-true cell = %d, want 9", outerTrue)
	}
}

// TestSupportAxisWideningIsIdempotent proves the operator reaches its own
// fixed point in one step. That is what bounds the head's partition: after one
// discharge the head carries no cycle-local distinction, so a later
// publication cannot reintroduce one through the same relation.
func TestSupportAxisWideningIsIdempotent(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	plan := selfScopeDischarge(t, composition, base.Scope(), nil, []guard.Atom{1})
	exact := writeStates(t, newWork(t, composition), binding, fixture, base, slot, []factWrite{{when: on, value: 4}, {when: off, value: 6}})
	once, ok := newWork(t, composition).Reindex(exact, plan)
	if !ok {
		t.Fatal("first discharge")
	}
	twice, ok := newWork(t, composition).Reindex(once, plan)
	if !ok {
		t.Fatal("second discharge")
	}
	work := newWork(t, composition)
	if !work.EqualUnder(once, twice) {
		t.Fatal("support-axis widening is not its own fixed point after one step")
	}
	root, rootOK := once.HandleAt(slot)
	if !rootOK {
		t.Fatal("discharged root")
	}
	unit := fixture.unit(t, 0)
	high, _, highOK := observedExactValue(binding, newWork(t, composition), root, unit, whole, func(guard.Atom) bool { return true })
	low, _, lowOK := observedExactValue(binding, newWork(t, composition), root, unit, whole, func(guard.Atom) bool { return false })
	if !highOK || !lowOK || high != low || high != 6 {
		t.Fatalf("discharged head = %d/%d, want the join 6 on both valuations", low, high)
	}
}

func mustAnd(t testing.TB, left, right support.Mask) support.Mask {
	t.Helper()
	result, ok := support.Intersect(left, right)
	if !ok {
		t.Fatal("conjoin cells")
	}
	return result
}
