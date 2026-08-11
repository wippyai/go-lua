package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestWorkCloseRevokesDynamicRootsAndDropsEpochStore proves the reclamation
// cut structurally, rather than asking the Go GC to happen at a particular
// time.  The initial composition root survives, while the dynamic root, its
// State provenance, and the slot work's typed store all fail closed at Close.
func TestWorkCloseRevokesDynamicRootsAndDropsEpochStore(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	next := writeState(t, work, binding, fixture, initial, slot, whole, 7)
	root, ok := next.HandleAt(slot)
	if !ok || !next.Valid() || !binding.ValidRoot(root) {
		t.Fatal("live dynamic root")
	}
	slotWork, ok := work.SlotWork(slot)
	typed, typedOK := slotWork.(*bindingWork[uint64, uint64])
	if !ok || !typedOK || typed.roots == nil || typed.epoch == nil {
		t.Fatal("epoch-local typed store")
	}
	if !work.Close() {
		t.Fatal("close work")
	}
	if next.Valid() {
		t.Fatal("closed epoch State remained valid")
	}
	if _, live := next.HandleAt(slot); live || binding.ValidRoot(root) {
		t.Fatal("closed epoch root remained reachable")
	}
	if typed.roots != nil || typed.epoch != nil || typed.binding != nil {
		t.Fatal("closed Work retained a typed root arena")
	}
	// Static initial roots are composition-owned and remain valid by design.
	if !initial.Valid() {
		t.Fatal("close revoked immutable initial roots")
	}
}

// TestRetainedWorkKeepsOnlyOneCompletedEpochUntilEviction proves the distinct
// completed-cache ownership state.  Retain makes the Work non-executable but
// leaves its immutable result readable; closing the lease is the sole
// eviction/revocation cut.
func TestRetainedWorkKeepsOnlyOneCompletedEpochUntilEviction(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	next := writeState(t, work, binding, fixture, initial, slot, whole, 11)
	root, ok := next.HandleAt(slot)
	if !ok {
		t.Fatal("dynamic root")
	}
	retained, ok := work.Retain()
	if !ok || !retained.Live() || !next.Valid() || !binding.ValidRoot(root) {
		t.Fatal("retain completed epoch")
	}
	if work.OwnsState(next) {
		t.Fatal("retained Work remained executable")
	}
	if !retained.Close() {
		t.Fatal("evict retained epoch")
	}
	if retained.Live() || next.Valid() || binding.ValidRoot(root) {
		t.Fatal("evicted completed epoch remained live")
	}
}

// TestRepeatedClosedWorkEpochsLeaveNoTypedRootStore is the bounded plateau
// law for cancellation/incomplete solves.  It does not measure GC: each pass
// keeps the closed SlotWork reachable and proves its root-store pointer was
// cleared, while an escaped root is rejected before the next epoch opens.
func TestRepeatedClosedWorkEpochsLeaveNoTypedRootStore(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	var prior carrier.RootHandle
	for round := uint64(1); round <= 64; round++ {
		work := newWork(t, composition)
		next := writeState(t, work, binding, fixture, initial, slot, whole, round)
		root, rootOK := next.HandleAt(slot)
		if !rootOK || !binding.ValidRoot(root) || prior == root {
			t.Fatalf("round %d dynamic root identity", round)
		}
		slotWork, slotOK := work.SlotWork(slot)
		typed, typedOK := slotWork.(*bindingWork[uint64, uint64])
		if !slotOK || !typedOK || typed.roots == nil {
			t.Fatalf("round %d root store", round)
		}
		if !work.Close() {
			t.Fatalf("round %d close", round)
		}
		if next.Valid() || binding.ValidRoot(root) || typed.roots != nil {
			t.Fatalf("round %d retained closed epoch root", round)
		}
		prior = root
	}
	// A fresh epoch still works after every prior root-store was reclaimed.
	fresh := newWork(t, composition)
	defer fresh.Close()
	next := writeState(t, fresh, binding, fixture, initial, slot, whole, 99)
	if !next.Valid() {
		t.Fatal("fresh epoch after closed sequence")
	}
}
