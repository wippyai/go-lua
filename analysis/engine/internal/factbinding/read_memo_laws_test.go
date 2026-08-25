package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// commitMemoValue changes one declared exact key through the normal typed
// patch/publication cut.  The returned State carries the newly issued root;
// tests below deliberately observe its values and regions rather than
// inspecting memo hit counters.
func commitMemoValue(t testing.TB, binding *Binding[uint64, uint64], work *carrier.Work, state carrier.State, target carrier.Target, region support.Mask, value uint64) carrier.State {
	t.Helper()
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(target, region, value) {
		t.Fatal("memo law write")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("memo law accept")
	}
	return commit(t, work, state, candidate)
}

func memoSlotWork(t testing.TB, work *carrier.Work, slot shape.Slot) carrier.SlotWork {
	t.Helper()
	slotWork, ok := work.SlotWork(slot)
	if !ok {
		t.Fatal("memo law slot work")
	}
	return slotWork
}

func assertStoredAbsentRows(t testing.TB, rows []observedRow, storedRegion, absentRegion support.Mask, stored uint64) {
	t.Helper()
	if len(rows) != 2 {
		t.Fatalf("exact rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if len(row.entries) != 1 {
			t.Fatal("exact memo row width")
		}
		entry := row.entries[0]
		value, present := entry.Read()
		switch {
		case row.region.Equal(storedRegion):
			if value != stored || !present {
				t.Fatalf("stored row = %d/%t, want %d/true", value, present, stored)
			}
		case row.region.Equal(absentRegion):
			if value != 0 || present {
				t.Fatalf("absent row = %d/%t, want 0/false", value, present)
			}
		default:
			t.Fatal("memo law emitted an unexpected support region")
		}
	}
}

func assertOneMemoRow(t testing.TB, rows []observedRow, region support.Mask, value uint64, present bool) {
	t.Helper()
	if len(rows) != 1 || len(rows[0].entries) != 1 || !rows[0].region.Equal(region) {
		t.Fatalf("restricted exact rows = %#v, want one row over requested region", rows)
	}
	got, gotPresent := rows[0].entries[0].Read()
	if got != value || gotPresent != present {
		t.Fatalf("restricted exact row = %d/%t, want %d/%t", got, gotPresent, value, present)
	}
}

func TestObservationReadMemoKeysCommittedRootIdentity(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, off, _, _ := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	first := commitMemoValue(t, binding, work, initial, declared.target[0], on, 7)
	firstRoot, ok := first.HandleAt(slot)
	if !ok {
		t.Fatal("first root")
	}
	assertStoredAbsentRows(t, observe(t, binding, memoSlotWork(t, work, slot), firstRoot, declared.exact[0], whole), on, off, 7)

	second := commitMemoValue(t, binding, work, first, declared.target[0], on, 8)
	secondRoot, ok := second.HandleAt(slot)
	if !ok || secondRoot == firstRoot {
		t.Fatal("changed fact did not publish a distinct root")
	}
	assertStoredAbsentRows(t, observe(t, binding, memoSlotWork(t, work, slot), secondRoot, declared.exact[0], whole), on, off, 8)
	if !work.Close() {
		t.Fatal("close root-identity work")
	}
}

func TestObservationReadMemoKeysUnitAndKey(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	whole, onOne, offOne, onTwo, offTwo := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	state := commitMemoValue(t, binding, work, initial, declared.target[0], onOne, 7)
	state = commitMemoValue(t, binding, work, state, declared.target[1], onTwo, 13)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("key memo root")
	}
	slotWork := memoSlotWork(t, work, slot)
	assertStoredAbsentRows(t, observe(t, binding, slotWork, root, declared.exact[0], whole), onOne, offOne, 7)
	// The second exact Unit has a different declared key and value under the
	// same immutable root. Reusing the first key's partition would expose 7
	// over atom 1 instead of this row's 13 over atom 2.
	assertStoredAbsentRows(t, observe(t, binding, slotWork, root, declared.exact[1], whole), onTwo, offTwo, 13)
	if !work.Close() {
		t.Fatal("close key memo work")
	}
}

func TestObservationReadMemoKeysWithinRegion(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, off, _, _ := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	state := commitMemoValue(t, binding, work, initial, declared.target[0], on, 7)
	root, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("within memo root")
	}
	slotWork := memoSlotWork(t, work, slot)
	assertStoredAbsentRows(t, observe(t, binding, slotWork, root, declared.exact[0], whole), on, off, 7)
	assertOneMemoRow(t, observe(t, binding, slotWork, root, declared.exact[0], on), on, 7, true)
	assertOneMemoRow(t, observe(t, binding, slotWork, root, declared.exact[0], off), off, 0, false)
	if !work.Close() {
		t.Fatal("close within memo work")
	}
}

func TestObservationReadMemoClearsAtCloseAndNewWork(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	oldWork := newWork(t, composition)
	oldState := commitMemoValue(t, binding, oldWork, initial, declared.target[0], whole, 7)
	oldRoot, ok := oldState.HandleAt(slot)
	if !ok {
		t.Fatal("old epoch root")
	}
	assertOneMemoRow(t, observe(t, binding, memoSlotWork(t, oldWork, slot), oldRoot, declared.exact[0], whole), whole, 7, true)
	if !oldWork.Close() {
		t.Fatal("close old memo epoch")
	}

	// A fresh Work reuses compact root IDs but carries a distinct RootEpoch.
	// Its changed value must be observed, proving that neither a Work-global
	// entry nor a stale old epoch can satisfy the new read.
	freshWork := newWork(t, composition)
	newState := commitMemoValue(t, binding, freshWork, initial, declared.target[0], whole, 9)
	newRoot, ok := newState.HandleAt(slot)
	if !ok || newRoot == oldRoot {
		t.Fatal("new epoch did not issue a distinct root handle")
	}
	assertOneMemoRow(t, observe(t, binding, memoSlotWork(t, freshWork, slot), newRoot, declared.exact[0], whole), whole, 9, true)
	if !freshWork.Close() {
		t.Fatal("close new memo epoch")
	}
}

func TestObservationReadMemoCapacityIsDeclaredKeyBoundAndEvicts(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, off, _, _ := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork := memoSlotWork(t, work, slot)
	typed, ok := slotWork.(*bindingWork[uint64, uint64])
	if !ok || typed == nil {
		t.Fatal("typed memo owner")
	}
	bound := binding.declaredKeyCount()
	if bound == 0 || cap(typed.readMemo.entries) > bound || len(typed.readMemo.entries) > bound {
		t.Fatalf("read memo capacity = len %d cap %d, want bound %d", len(typed.readMemo.entries), cap(typed.readMemo.entries), bound)
	}

	// Each commit issues a distinct root, while the read alternates among the
	// whole support and both proper regions. A declared key holds one entry,
	// so a new root or region at that coordinate evicts the old one and
	// recomputes; the observed current value and exact requested region
	// remain the authority for every iteration.
	state := initial
	regions := []support.Mask{whole, on, off}
	for value := uint64(1); value <= 64; value++ {
		state = commitMemoValue(t, binding, work, state, declared.target[0], whole, value)
		root, rootOK := state.HandleAt(slot)
		if !rootOK {
			t.Fatalf("root at value %d", value)
		}
		within := regions[(value-1)%uint64(len(regions))]
		assertOneMemoRow(t, observe(t, binding, slotWork, root, declared.exact[0], within), within, value, true)
		if cap(typed.readMemo.entries) > bound || len(typed.readMemo.entries) > bound {
			t.Fatalf("read memo exceeded Unit bound at value %d: len %d cap %d bound %d", value, len(typed.readMemo.entries), cap(typed.readMemo.entries), bound)
		}
	}
	if !work.Close() {
		t.Fatal("close bounded memo work")
	}
}

// TestObservationReadMemoIsAddressedByKeyCoordinate states that the memo is
// read by coordinate. A declared key's pieces live at that key's position in
// the observed Unit's frozen key vector, so one read examines one entry. A
// memo that examines more entries than the reads offered to it is searching a
// table whose address it already holds, and that search grows with the
// Binding's sealed inventory while the read it caches does not.
func TestObservationReadMemoIsAddressedByKeyCoordinate(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, _, _, _ := observationMasks(t, manager)
	binding, initial, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	slotWork := memoSlotWork(t, work, slot)

	// A branched value makes the summary traverse its declared keys twice:
	// once to probe for a constant vector and once to build the groups. The
	// second traversal is exactly what the memo exists to serve.
	state := commitMemoValue(t, binding, work, initial, declared.target[0], on, 5)
	root, rootOK := state.HandleAt(slot)
	if !rootOK {
		t.Fatal("committed root")
	}
	DbgFactBindingReset()
	if rows := observe(t, binding, slotWork, root, declared.summary, whole); len(rows) == 0 {
		t.Fatal("summary observation produced no row")
	}
	counters := DbgFactBinding()
	if counters.ReadMemoReads == 0 {
		t.Fatal("the summary observation offered no declared-key read to the memo")
	}
	if counters.ReadMemoProbes != counters.ReadMemoReads {
		t.Errorf("read memo examined %d entries for %d reads: the entry is addressed by the key's coordinate, so a read examines exactly its own entry",
			counters.ReadMemoProbes, counters.ReadMemoReads)
	}
	if !work.Close() {
		t.Fatal("close coordinate memo work")
	}
}
