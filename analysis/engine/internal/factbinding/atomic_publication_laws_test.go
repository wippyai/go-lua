package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestJoinResultEqualToRightReportsLeftDelta proves that a lawful recurrence
// result retains the right semantic value while reporting the exact
// predecessor-left-to-output dependency region.
func TestJoinResultEqualToRightReusesRightRootAndReportsLeftDelta(t *testing.T) {
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
	leftRoot, ok := left.HandleAt(slot)
	if !ok {
		t.Fatal("left root")
	}
	rightRoot, ok := right.HandleAt(slot)
	if !ok || leftRoot == rightRoot {
		t.Fatal("distinct right root")
	}
	work := newWork(t, composition)
	next, changes, ok := work.Merge3Under(carrier.Join, left, right, composition.AllMergeScope())
	if !ok {
		t.Fatal("lawful join")
	}
	after, ok := next.HandleAt(slot)
	if !ok || after != rightRoot {
		t.Fatal("join result did not reuse exact right root")
	}
	if changes.Count() != 1 || changes.FactorCount() != 1 || !support.Empty(changes.Added()) || !support.Empty(changes.Removed()) {
		t.Fatalf("join ChangeSet shape = rows:%d added:%t removed:%t", changes.Count(), support.Empty(changes.Added()), support.Empty(changes.Removed()))
	}
	row, present := changes.At(0)
	target := fixture.target(t, 0, carrier.StrongTarget)
	units, declared := composition.TargetNotifications(slot, target)
	if !present || !declared || len(units) != 1 || !row.Unit().Same(units[0]) || !row.Region().Equal(whole) {
		t.Fatal("join ChangeSet was not the exact left-to-right unit region")
	}
	factor, present := changes.FactorAt(0)
	if !present || factor.Slot() != slot || !factor.Region().Equal(whole) {
		t.Fatal("join ChangeSet was not the exact left-to-right Factor region")
	}
}

// TestPatchIsBoundToItsProducingWork closes the evaluator-provenance hole:
// a valid candidate cannot cross between two Work instances for the same
// Composition. Rejection consumes it.
func TestPatchIsBoundToItsProducingWork(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, _, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	producer, receiver := newWork(t, composition), newWork(t, composition)
	stage := binding.Begin(producer, state)
	if stage == nil || !stage.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("stage")
	}
	candidate, ok := stage.Accept(producer)
	if !ok {
		t.Fatal("accept")
	}
	if _, _, committed := receiver.Commit(state, []carrier.Patch{candidate}); committed {
		t.Fatal("foreign Work committed candidate")
	}
	if _, _, committed := producer.Commit(state, []carrier.Patch{candidate}); committed {
		t.Fatal("rejected foreign-work candidate remained reusable")
	}
}

func TestPatchAcceptFailureConsumesItsSoleStageCandidate(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, _, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	staged := binding.Begin(work, state)
	if staged == nil || !staged.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("stage")
	}
	if _, accepted := staged.Accept(nil); accepted {
		t.Fatal("nil Work accepted stage candidate")
	}
	if staged.Discard() {
		t.Fatal("failed Accept left the stage candidate open")
	}
	if replacement := binding.Begin(work, state); replacement == nil || !replacement.Discard() {
		t.Fatal("failed Accept damaged the binding stage lifecycle")
	}
}

// TestCheckpointAbandonsFusedFDDMergeBeforePublication places cancellation
// after the carrier has entered a typed merge, rather than only at an
// executor boundary. The output Plane and its pending root remain private to
// bindingWork; the two immutable input States must be the only visible roots.
func TestCheckpointAbandonsFusedFDDMergeBeforePublication(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	write := func(value0, value1 uint64) carrier.State {
		work := newWork(t, composition)
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, value0) || !patch.Write(fixture.target(t, 1, carrier.StrongTarget), whole, value1) {
			t.Fatal("write")
		}
		accepted, ok := patch.Accept(work)
		if !ok {
			t.Fatal("accept")
		}
		return commit(t, work, state, accepted)
	}
	left, right := write(1, 2), write(3, 4)
	leftRoot, _ := left.HandleAt(slot)
	rightRoot, _ := right.HandleAt(slot)
	work := newWork(t, composition)
	checks := 0
	if !work.SetCheckpoint(func() bool {
		checks++
		return checks < 8
	}) {
		t.Fatal("checkpoint")
	}
	if _, _, merged := work.Merge3Under(carrier.Join, left, right, composition.AllMergeScope()); merged {
		t.Fatal("cancelled fused merge published a result")
	}
	if checks < 8 {
		t.Fatalf("checkpoint did not reach fused traversal, checks=%d", checks)
	}
	if root, _ := left.HandleAt(slot); root != leftRoot {
		t.Fatal("cancelled merge changed left visible root")
	}
	if root, _ := right.HandleAt(slot); root != rightRoot {
		t.Fatal("cancelled merge changed right visible root")
	}
}
