package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestTypedCarryExclusionReplacesStaleTargetAndRetainsSibling is the
// red-first typed half of target-scoped carry exclusion. The carried source
// contains stale A and sibling B over the whole support. Its sealed carry
// surface excludes A, while a concrete Patch authors only A on the `on`
// region. Closing must therefore retain the patch's A value on `on`, retain B,
// and erase stale A on `off`.
func TestTypedCarryExclusionReplacesStaleTargetAndRetainsSibling(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
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
		t.Fatal("support regions")
	}

	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	strongA := fixture.target(t, 0, carrier.StrongTarget)
	strongB := fixture.target(t, 1, carrier.StrongTarget)
	writePlan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil)
	if !ok {
		t.Fatal("write plan")
	}
	source := carrier.ContributionSource{Slot: slot, Input: 0}
	carryPlan, ok := composition.SealContribution(1, []shape.Slot{slot}, []carrier.ContributionSource{source})
	if !ok {
		t.Fatal("carry plan")
	}
	carryPlan, ok = carryPlan.SealCarryExclusions(map[carrier.ContributionSource][]carrier.Target{
		source: {strongA},
	})
	if !ok {
		t.Fatal("carry exclusion")
	}
	work := newWork(t, composition)

	// The predecessor has both the stale target A and its carried sibling B.
	base, ok := work.BeginContribution(writePlan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin source")
	}
	seed := binding.Begin(work, base.State())
	if seed == nil || !seed.Write(strongA, whole, 9) || !seed.Write(strongB, whole, 8) {
		t.Fatal("seed stale/sibling source")
	}
	seedPatch, ok := seed.Accept(work)
	if !ok {
		t.Fatal("accept source")
	}
	sourceContribution, ok := work.FinishContribution(base, []carrier.Patch{seedPatch})
	if !ok {
		t.Fatal("finish source")
	}
	sourceRule, ok := work.AsRuleContribution(sourceContribution)
	if !ok {
		t.Fatal("source rule")
	}
	sourcePoint, ok := work.PointStateFromRuleContribution(sourceRule)
	if !ok {
		t.Fatal("source point")
	}
	sourceRoot, ok := sourcePoint.HandleAt(slot)
	if !ok {
		t.Fatal("source root")
	}
	if got, present, valid := observedExactValue(binding, work, sourceRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("stale predecessor A = %d/%t/%t, want 9/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, sourceRoot, fixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 8 {
		t.Fatalf("sibling predecessor B = %d/%t/%t, want 8/true/true", got, present, valid)
	}

	// The canonical carry surface has excluded A but retained sibling B. Read
	// it through the nominal publication boundary rather than inspecting the
	// carrier's private coverage storage.
	carryBase, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{sourcePoint}, whole)
	if !ok {
		t.Fatal("begin carry-only contribution")
	}
	carryOnly, ok := work.FinishRuleContribution(carryBase, nil)
	if !ok {
		t.Fatal("finish carry-only contribution")
	}
	carryPoint, ok := work.PointStateFromRuleContribution(carryOnly)
	if !ok {
		t.Fatal("carry-only point")
	}
	emptyPoint, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("empty point")
	}
	carryCoverage, ok := work.CoverageChangesPointStates(emptyPoint, carryPoint)
	if !ok || carryCoverage.TargetCount() != 1 {
		t.Fatalf("canonical carry surface: ok=%t targets=%d, want one sibling target", ok, carryCoverage.TargetCount())
	}
	carryRow, ok := carryCoverage.TargetAt(0)
	if !ok || !carryRow.Target().Same(strongB) || !carryRow.Region().Equal(whole) {
		t.Fatal("canonical carry surface did not exclude A and retain B")
	}

	// A's concrete patch covers only `on`, leaving stale A on `off` in the
	// candidate root. The close must erase that latent branch while preserving
	// the patch and the sibling carry.
	patchedBase, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{sourcePoint}, whole)
	if !ok {
		t.Fatal("begin patched contribution")
	}
	patch := binding.Begin(work, patchedBase.State())
	if patch == nil || !patch.Write(strongA, on, 4) {
		t.Fatal("author A replacement")
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept A replacement")
	}
	result, ok := work.FinishRuleContribution(patchedBase, []carrier.Patch{publication})
	if !ok || !result.Valid() {
		t.Fatal("close patched contribution")
	}
	resultRoot, ok := result.HandleAt(slot)
	if !ok {
		t.Fatal("result root")
	}
	if got, present, valid := observedExactValue(binding, work, resultRoot, fixture.unit(t, 0), on, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("patched A on = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, resultRoot, fixture.unit(t, 0), off, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("stale A off survived close = %d/%t/%t, want 0/false/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, resultRoot, fixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 8 {
		t.Fatalf("sibling B after close = %d/%t/%t, want 8/true/true", got, present, valid)
	}
}
