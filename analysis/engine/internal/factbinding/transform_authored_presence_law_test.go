package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
)

// TestTransformAuthoredRowsFollowCanonicalCarryCoverage proves that carry
// authorship follows the source contribution's retained coverage: true
// Absent remains uncovered, while an explicit sparse Default remains authored.
func TestTransformAuthoredRowsFollowCanonicalCarryCoverage(t *testing.T) {
	binding, state, slot, composition, whole, on, _, declared := changeFixture(t)
	writePlan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil)
	if !ok {
		t.Fatal("write plan")
	}
	source := carrier.ContributionSource{Slot: slot, Input: 0}
	carryPlan, ok := composition.SealContribution(1, []shape.Slot{slot}, []carrier.ContributionSource{source})
	if !ok {
		t.Fatal("carry plan")
	}
	work := newWork(t, composition)
	closure, ok := binding.TransformClosure([]carrier.Target{declared.strong[0]})
	if !ok {
		t.Fatal("transform closure")
	}
	emptyContribution, ok := work.EmptyContribution(state)
	if !ok {
		t.Fatal("empty contribution")
	}
	emptyPoint := closedPointOf(t, work, emptyContribution)
	explicitDefault := finishContributionAt(t, work, writePlan, composition.Scope(), binding, declared.strong[0], whole, 0)
	explicitPoint := closedPointOf(t, work, explicitDefault)

	beginCarry := func(input carrier.PointState) (carrier.RuleContributionBase, carrier.SlotCoverage) {
		base, began := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{input}, whole)
		if !began {
			t.Fatal("begin carry")
		}
		coverage, covered := work.RuleContributionCarrySlotCoverage(base, source)
		if !covered {
			t.Fatal("carry coverage")
		}
		return base, coverage
	}

	absentBase, absentCoverage := beginCarry(emptyPoint)
	absent := binding.Begin(work, absentBase.State())
	if absent == nil || !absent.Transform(closure, absentCoverage, whole, func(value uint64) (uint64, bool) {
		return value, true
	}) {
		t.Fatal("absent transform")
	}
	if len(absent.authored) != 0 {
		t.Fatal("Absent transformed key acquired authored coverage")
	}
	absentPatch, accepted := absent.Accept(work)
	if !accepted {
		t.Fatal("absent transform accept")
	}
	absentRule, finished := work.FinishRuleContribution(absentBase, []carrier.Patch{absentPatch})
	if !finished {
		t.Fatal("absent transform finish")
	}
	absentPoint, pointOK := work.PointStateFromRuleContribution(absentRule)
	if !pointOK {
		t.Fatal("absent transform point")
	}
	absentChanges, changesOK := work.CoverageChangesPointStates(emptyPoint, absentPoint)
	if !changesOK || absentChanges.Count() != 0 {
		t.Fatalf("Absent transform coverage changes = %d, want 0", absentChanges.Count())
	}

	defaultBase, defaultCoverage := beginCarry(explicitPoint)
	defaultPatch := binding.Begin(work, defaultBase.State())
	if defaultPatch == nil || !defaultPatch.Transform(closure, defaultCoverage, whole, func(value uint64) (uint64, bool) {
		return value, true
	}) {
		t.Fatal("explicit Default transform")
	}
	if len(defaultPatch.authored) != 1 || !defaultPatch.authored[0].Region().Equal(whole) {
		t.Fatal("explicit Default lost canonical authored coverage")
	}
	defaultPublication, accepted := defaultPatch.Accept(work)
	if !accepted {
		t.Fatal("explicit Default transform accept")
	}
	defaultRule, finished := work.FinishRuleContribution(defaultBase, []carrier.Patch{defaultPublication})
	if !finished {
		t.Fatal("explicit Default transform finish")
	}
	defaultPoint, pointOK := work.PointStateFromRuleContribution(defaultRule)
	if !pointOK {
		t.Fatal("explicit Default transform point")
	}
	defaultChanges, changesOK := work.CoverageChangesPointStates(emptyPoint, defaultPoint)
	if !changesOK || defaultChanges.Count() != 1 {
		t.Fatalf("explicit Default transform coverage changes = %d, want 1", defaultChanges.Count())
	}
	row, rowOK := defaultChanges.TargetAt(0)
	if !rowOK || !row.Target().Same(declared.strong[0]) || !row.Region().Equal(whole) {
		t.Fatal("explicit Default transformed row")
	}

	clippedBase, clippedCoverage := beginCarry(explicitPoint)
	clipped := binding.Begin(work, clippedBase.State())
	if clipped == nil || !clipped.Transform(closure, clippedCoverage, on, func(value uint64) (uint64, bool) {
		return value, true
	}) {
		t.Fatal("guard-clipped transform")
	}
	if len(clipped.authored) != 1 || !clipped.authored[0].Region().Equal(on) {
		t.Fatal("guard clip did not restrict authored coverage")
	}
	clippedPublication, accepted := clipped.Accept(work)
	if !accepted {
		t.Fatal("guard-clipped transform accept")
	}
	clippedRule, finished := work.FinishRuleContribution(clippedBase, []carrier.Patch{clippedPublication})
	if !finished {
		t.Fatal("guard-clipped transform finish")
	}
	clippedPoint, pointOK := work.PointStateFromRuleContribution(clippedRule)
	if !pointOK {
		t.Fatal("guard-clipped transform point")
	}
	if !clippedPoint.Valid() || !clippedRule.Valid() {
		t.Fatal("guard-clipped transform result")
	}
}

// TestTransformExcludesSourceCoverageOutsideTheSealedClosure proves that a
// valid source row belonging to another member's closure is authenticated but
// excluded from this member's transformed authored projection. The carry
// remainder remains unchanged.
func TestTransformExcludesSourceCoverageOutsideTheSealedClosure(t *testing.T) {
	binding, _, slot, composition, whole, _, _, declared := changeFixture(t)
	writePlan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil)
	if !ok {
		t.Fatal("write plan")
	}
	source := carrier.ContributionSource{Slot: slot, Input: 0}
	carryPlan, ok := composition.SealContribution(1, []shape.Slot{slot}, []carrier.ContributionSource{source})
	if !ok {
		t.Fatal("carry plan")
	}
	work := newWork(t, composition)
	outside := finishContributionAt(t, work, writePlan, composition.Scope(), binding, declared.strong[1], whole, 1)
	outsidePoint := closedPointOf(t, work, outside)
	base, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{outsidePoint}, whole)
	if !ok {
		t.Fatal("begin outside carry")
	}
	coverage, ok := work.RuleContributionCarrySlotCoverage(base, source)
	if !ok {
		t.Fatal("outside carry coverage")
	}
	closure, ok := binding.TransformClosure([]carrier.Target{declared.strong[0]})
	if !ok {
		t.Fatal("transform closure")
	}
	patch := binding.Begin(work, base.State())
	if patch == nil {
		t.Fatal("outside transform patch")
	}
	if !patch.Transform(closure, coverage, whole, func(value uint64) (uint64, bool) {
		return value, true
	}) {
		t.Fatal("outside source projection")
	}
	publication, accepted := patch.Accept(work)
	if !accepted {
		t.Fatal("outside transform accept")
	}
	result, finished := work.FinishRuleContribution(base, []carrier.Patch{publication})
	if !finished {
		t.Fatal("outside transform finish")
	}
	resultPoint, pointOK := work.PointStateFromRuleContribution(result)
	if !pointOK || !resultPoint.Valid() {
		t.Fatal("outside transform point")
	}
	changes, changesOK := work.CoverageChangesPointStates(outsidePoint, resultPoint)
	if !changesOK || changes.Count() != 0 {
		t.Fatalf("outside source projection changed carry remainder: changes=%d/%t", changes.Count(), changesOK)
	}
}
