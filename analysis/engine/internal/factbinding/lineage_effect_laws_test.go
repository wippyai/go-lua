package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

const (
	lineageRefuted uint64 = 1
	lineageProven  uint64 = 2
	lineageUnknown uint64 = 3
)

type lineageLawFixture struct {
	binding     *Binding[uint64, uint64]
	work        *carrier.Work
	composition *carrier.Composition
	slot        shape.Slot
	plan        carrier.ContributionPlan
	carryPlan   carrier.ContributionPlan
	source      carrier.PointState
	target      carrier.Target
	unit        carrier.Unit
	whole       support.Mask
	on          support.Mask
	off         support.Mask
}

func lineageLawConfig() testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      1,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			switch {
			case left == 0:
				return right
			case right == 0:
				return left
			case left == right:
				return left
			default:
				return lineageUnknown
			}
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool {
			return left == right || left == 0 || right == lineageUnknown
		},
	}
}

func newLineageLawFixture(t testing.TB) lineageLawFixture {
	t.Helper()
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on support")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off support")
	}
	whole := transportWhole(t, manager)
	binding, _, slot, composition, fixture := bindingState(t, manager, lineageLawConfig(), whole)
	plan := compositionPlan(t, composition)
	carrySource := carrier.ContributionSource{Slot: slot, Input: 0}
	carryPlan, ok := composition.SealContribution(1, []shape.Slot{slot}, []carrier.ContributionSource{carrySource})
	if !ok {
		t.Fatal("carry plan")
	}
	work := newWork(t, composition)
	t.Cleanup(func() { work.Close() })
	sourceValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, lineageRefuted)
	sourceRule, ok := work.AsRuleContribution(sourceValue)
	if !ok {
		t.Fatal("source rule")
	}
	source, ok := work.PointStateFromRuleContribution(sourceRule)
	if !ok {
		t.Fatal("source point")
	}
	target := fixture.target(t, 0, carrier.StrongTarget)
	unit := fixture.unit(t, 0)
	return lineageLawFixture{binding: binding, work: work, composition: composition, slot: slot, plan: plan, carryPlan: carryPlan, source: source, target: target, unit: unit, whole: whole, on: on, off: off}
}

func lineageEffectRule(t testing.TB, fixture lineageLawFixture, region support.Mask, value uint64) carrier.RuleContribution {
	t.Helper()
	source := carrier.ContributionSource{Slot: fixture.slot, Input: 0}
	base, ok := fixture.work.BeginRuleContribution(fixture.carryPlan, fixture.composition.Scope(), []carrier.PointState{fixture.source}, fixture.whole)
	if !ok {
		t.Fatal("begin effect rule")
	}
	coverage, ok := fixture.work.RuleContributionCarrySlotCoverage(base, source)
	if !ok {
		t.Fatal("effect source coverage")
	}
	closure, ok := fixture.binding.TransformClosure([]carrier.Target{fixture.target})
	if !ok {
		t.Fatal("effect closure")
	}
	patch := fixture.binding.Begin(fixture.work, base.State())
	if patch == nil || !patch.Transform(closure, coverage, region, func(current uint64) (uint64, bool) {
		if current == 0 {
			return current, true
		}
		return value, true
	}) {
		t.Fatal("stage effect")
	}
	accepted, ok := patch.Accept(fixture.work)
	if !ok {
		t.Fatal("accept effect")
	}
	rule, ok := fixture.work.FinishRuleContribution(base, []carrier.Patch{accepted})
	if !ok {
		t.Fatal("finish effect")
	}
	return rule
}

func (fixture lineageLawFixture) foldRules(t testing.TB, rules ...carrier.RuleContribution) carrier.PointRHS {
	t.Helper()
	rhs, ok := fixture.work.PointRHSFromPointState(fixture.source)
	if !ok || !fixture.work.BeginPointRHSFold(fixture.source, rhs) {
		t.Fatal("begin lineage fold")
	}
	for _, rule := range rules {
		if !fixture.work.AddPointFoldRule(rule) {
			t.Fatal("add lineage rule")
		}
	}
	folded, _, ok := fixture.work.FinishPointRHSFold()
	if !ok {
		t.Fatal("finish lineage fold")
	}
	return folded
}

func assertLineageValue(t testing.TB, fixture lineageLawFixture, root carrier.RootHandle, region func(guard.Atom) bool, want uint64) {
	t.Helper()
	got, present, valid := observedExactValue(fixture.binding, fixture.work, root, fixture.unit, fixture.whole, region)
	if !valid || !present || got != want {
		t.Fatalf("lineage value = %d/%t/%t, want %d/true/true", got, present, valid, want)
	}
}

// TestPointFoldSameLineageBaselineEffectOverlapUsesEffect proves that a
// same-lineage effect masks its carried baseline before typed Join runs.
func TestPointFoldSameLineageBaselineEffectOverlapUsesEffect(t *testing.T) {
	fixture := newLineageLawFixture(t)
	effect := lineageEffectRule(t, fixture, fixture.on, lineageProven)
	folded := fixture.foldRules(t, effect)
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageProven)
	assertLineageValue(t, fixture, root, func(guard.Atom) bool { return false }, lineageRefuted)
}

// TestPointFoldSameLineageBaselineEffectDisjointPreservesPartition proves
// that masking is local: a disjoint effect leaves the baseline partition.
func TestPointFoldSameLineageBaselineEffectDisjointPreservesPartition(t *testing.T) {
	fixture := newLineageLawFixture(t)
	effect := lineageEffectRule(t, fixture, fixture.off, lineageProven)
	folded := fixture.foldRules(t, effect)
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageRefuted)
	assertLineageValue(t, fixture, root, func(guard.Atom) bool { return false }, lineageProven)
}

// TestPointFoldDistinctLineagesJoinOverlapUnknown proves that rows from
// distinct published point generations remain independent Join operands.
func TestPointFoldDistinctLineagesJoinOverlapUnknown(t *testing.T) {
	fixture := newLineageLawFixture(t)
	otherValue := finishContributionAt(t, fixture.work, fixture.plan, fixture.composition.Scope(), fixture.binding, fixture.target, fixture.whole, lineageProven)
	otherRule, ok := fixture.work.AsRuleContribution(otherValue)
	if !ok {
		t.Fatal("other rule")
	}
	otherPoint, ok := fixture.work.PointStateFromRuleContribution(otherRule)
	if !ok {
		t.Fatal("other point")
	}
	rhs, ok := fixture.work.PointRHSFromPointState(fixture.source)
	if !ok || !fixture.work.BeginPointRHSFold(fixture.source, rhs) || !fixture.work.AddPointFoldEnvironment(otherPoint) {
		t.Fatal("distinct-lineage fold")
	}
	folded, _, ok := fixture.work.FinishPointRHSFold()
	if !ok {
		t.Fatal("finish distinct-lineage fold")
	}
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageUnknown)
}

// TestPointFoldSameLineageEffectsJoinOverlapUnknown proves that overlapping
// effects retain both operands even when they share the source lineage.
func TestPointFoldSameLineageEffectsJoinOverlapUnknown(t *testing.T) {
	fixture := newLineageLawFixture(t)
	refuted := lineageEffectRule(t, fixture, fixture.whole, lineageRefuted)
	proven := lineageEffectRule(t, fixture, fixture.whole, lineageProven)
	folded := fixture.foldRules(t, refuted, proven)
	root, ok := folded.HandleAt(0)
	if !ok {
		t.Fatal("folded root")
	}
	assertLineageValue(t, fixture, root, func(atom guard.Atom) bool { return atom == 1 }, lineageUnknown)
}
