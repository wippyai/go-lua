package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestLineageCoverageUnionMergeRetainsBaselineEffectSameTarget proves that
// provenance metadata is part of coverage identity: a baseline and an effect
// on the same Target remain two rows through both coverage join paths.
func TestLineageCoverageUnionMergeRetainsBaselineEffectSameTarget(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	seedCoverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	seed, ok := work.admitContribution(initial, seedCoverage)
	if !ok {
		t.Fatal("seed contribution")
	}
	rule, ok := work.AsRuleContribution(seed)
	if !ok {
		t.Fatal("seed rule")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("seed point")
	}
	lineage := point.lineage
	baseline := point.coverage.slot(0).targets[0]
	effect := TargetRegion{target: operation.target, region: whole, lineage: lineage, role: CoverageEffect}
	left := slotCoverage{targets: []TargetRegion{baseline}}
	right := slotCoverage{targets: []TargetRegion{effect}}
	if work.slotCoverageContainsProvenance(left, right) || work.slotCoverageContainsProvenance(right, left) {
		t.Fatal("metadata-mismatched rows were falsely contained")
	}
	assertRows := func(name string, coverage slotCoverage) {
		t.Helper()
		if len(coverage.targets) != 2 {
			t.Fatalf("%s row count = %d, want 2", name, len(coverage.targets))
		}
		if coverage.targets[0].Role() != CoverageBaseline || coverage.targets[1].Role() != CoverageEffect {
			t.Fatalf("%s roles = %v/%v, want baseline/effect", name, coverage.targets[0].Role(), coverage.targets[1].Role())
		}
		for index, row := range coverage.targets {
			if !row.Target().Same(operation.target) || row.Lineage() != lineage || !row.Region().Equal(whole) {
				t.Fatalf("%s row %d lost Target/lineage/region", name, index)
			}
		}
	}
	union, ok := work.unionSlotCoverage(left, right)
	if !ok {
		t.Fatal("coverage union")
	}
	assertRows("union", union)
	merged, ok := work.mergeSlotCoverage(left, right, whole)
	if !ok {
		t.Fatal("coverage merge")
	}
	assertRows("merge", merged)
}

// TestPublishPointStateCanonicalizesRemappedDuplicateRows proves that a
// publication consumes incoming provenance before installing the new
// baseline. A baseline/effect pair on one Target therefore becomes one
// canonical baseline row rather than two identical rows in the next point.
func TestPublishPointStateCanonicalizesRemappedDuplicateRows(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	seedCoverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	seed, ok := work.admitContribution(initial, seedCoverage)
	if !ok {
		t.Fatal("seed contribution")
	}
	rule, ok := work.AsRuleContribution(seed)
	if !ok {
		t.Fatal("seed rule")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("seed point")
	}
	baseline := point.coverage.slot(0).targets[0]
	effect := baseline.WithLineage(point.lineage, CoverageEffect)
	input := contributionCoverage{composition: composition, slots: []slotCoverage{{targets: []TargetRegion{baseline, effect}}}}
	input.occupied.Set(0)
	published, ok := work.publishPointState(point.state, input, true)
	if !ok {
		t.Fatal("publish duplicate coverage")
	}
	rows := published.coverage.slot(0).targets
	if len(rows) != 1 || rows[0].Role() != CoverageBaseline || rows[0].Lineage() == point.lineage {
		t.Fatalf("published rows = %d/%v, want one fresh baseline", len(rows), rows)
	}
}

// TestLineageCoverageCanonicalMetadataOrderIsInputIndependent proves that
// reversing duplicate-Target inputs cannot change baseline/effect ordering or
// the resulting union/merge representation.
func TestLineageCoverageCanonicalMetadataOrderIsInputIndependent(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	seedCoverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	seed, ok := work.admitContribution(initial, seedCoverage)
	if !ok {
		t.Fatal("seed contribution")
	}
	rule, ok := work.AsRuleContribution(seed)
	if !ok {
		t.Fatal("seed rule")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("seed point")
	}
	baseline := point.coverage.slot(0).targets[0]
	effect := baseline.WithLineage(point.lineage, CoverageEffect)
	ordered, ok := work.canonicalCoverage([]TargetRegion{baseline, effect}, whole)
	if !ok {
		t.Fatal("ordered canonical coverage")
	}
	reversed, ok := work.canonicalCoverage([]TargetRegion{effect, baseline}, whole)
	if !ok {
		t.Fatal("reversed canonical coverage")
	}
	if !sameSlotCoverageProvenance(ordered, reversed) || len(ordered.targets) != 2 || ordered.targets[0].Role() != CoverageBaseline || ordered.targets[1].Role() != CoverageEffect {
		t.Fatalf("canonical order differs: ordered=%v reversed=%v", ordered.targets, reversed.targets)
	}
	unionForward, ok := work.unionSlotCoverage(slotCoverage{targets: []TargetRegion{baseline}}, slotCoverage{targets: []TargetRegion{effect}})
	if !ok {
		t.Fatal("forward union")
	}
	unionReverse, ok := work.unionSlotCoverage(slotCoverage{targets: []TargetRegion{effect}}, slotCoverage{targets: []TargetRegion{baseline}})
	if !ok {
		t.Fatal("reverse union")
	}
	if !sameSlotCoverageProvenance(unionForward, unionReverse) {
		t.Fatal("union order changed metadata order")
	}
	mergeForward, ok := work.mergeSlotCoverage(slotCoverage{targets: []TargetRegion{baseline}}, slotCoverage{targets: []TargetRegion{effect}}, whole)
	if !ok {
		t.Fatal("forward merge")
	}
	mergeReverse, ok := work.mergeSlotCoverage(slotCoverage{targets: []TargetRegion{effect}}, slotCoverage{targets: []TargetRegion{baseline}}, whole)
	if !ok {
		t.Fatal("reverse merge")
	}
	if !sameSlotCoverageProvenance(mergeForward, mergeReverse) {
		t.Fatal("merge order changed metadata order")
	}
}
