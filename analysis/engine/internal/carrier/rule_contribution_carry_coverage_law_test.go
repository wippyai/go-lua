package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestRuleContributionCarrySlotCoverageAuthenticatesTheSealedSource proves
// that transformed-carry execution can borrow only the exact retained rows
// issued by its live RuleContributionBase. It neither resolves a root nor
// manufactures a typed coverage surface for an unsealed source.
func TestRuleContributionCarrySlotCoverageAuthenticatesTheSealedSource(t *testing.T) {
	manager, err := guard.New(nil)
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
	initial, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	carrySource := ContributionSource{Slot: 0, Input: 0}
	carryPlan, ok := composition.SealContribution(1, nil, []ContributionSource{carrySource})
	if !ok {
		t.Fatal("carry plan")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()

	coverage := newContributionCoverage(composition, []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}})
	source, ok := work.admitContribution(initial, coverage)
	if !ok {
		t.Fatal("source contribution")
	}
	rule, ok := work.AsRuleContribution(source)
	if !ok {
		t.Fatal("source rule contribution")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("source point")
	}
	base, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []PointState{point}, whole)
	if !ok {
		t.Fatal("rule contribution base")
	}
	rows, ok := work.RuleContributionCarrySlotCoverage(base, carrySource)
	if !ok || rows.Count() != 1 {
		t.Fatal("sealed carry coverage")
	}
	row, ok := rows.At(0)
	if !ok || !row.Target().Same(operation.target) || !row.Region().Equal(whole) {
		t.Fatal("carry coverage row")
	}
	if rows.value != &base.value.inputs[0].coverage.slots[0] {
		t.Fatal("carry coverage was copied instead of retained")
	}

	if _, valid := work.RuleContributionCarrySlotCoverage(base, ContributionSource{Slot: 1, Input: 0}); valid {
		t.Fatal("unsealed carry slot authenticated")
	}
	if _, valid := work.RuleContributionCarrySlotCoverage(base, ContributionSource{Slot: 0, Input: 1}); valid {
		t.Fatal("unsealed carry input authenticated")
	}
	if _, valid := work.RuleContributionCarrySlotCoverage(RuleContributionBase{}, carrySource); valid {
		t.Fatal("foreign base authenticated")
	}
	if !work.AbortRuleContribution(base, nil) {
		t.Fatal("abort base")
	}
	if _, valid := work.RuleContributionCarrySlotCoverage(base, carrySource); valid {
		t.Fatal("consumed base authenticated")
	}
}
