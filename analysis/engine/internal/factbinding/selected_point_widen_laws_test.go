package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestMergeSelectedPointStateWidenAcceptsAscendingCachedExact proves the
// localized recurrence contract used when a region refresh has an old exact
// R0 and a fresh exact R1. R1 is built canonically as E+B, while the selected
// operand is that same fresh R1. The selected PointState may grow support
// from X's on-fiber support to the whole guard universe; the carrier must
// still preserve X as an upper-bound predecessor, publish exactly R1's
// compact authored surface, and agree with the canonical X+E+B fold.
func TestMergeSelectedPointStateWidenAcceptsAscendingCachedExact(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("off")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}

	binding, _, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	plan := compositionPlan(t, composition)
	widenScope, ok := composition.SealWidening([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("widen scope")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	toRule := func(value carrier.Contribution) carrier.RuleContribution {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("rule")
		}
		return rule
	}
	toPoint := func(rule carrier.RuleContribution) carrier.PointState {
		t.Helper()
		point, ok := work.PointStateFromRuleContribution(rule)
		if !ok {
			t.Fatal("point")
		}
		return point
	}
	toRHS := func(rule carrier.RuleContribution) carrier.PointRHS {
		t.Helper()
		rhs, ok := work.PointRHSFromRuleContribution(rule)
		if !ok {
			t.Fatal("RHS")
		}
		return rhs
	}

	// X is the current published point. Its semantic support/C surface is on,
	// but its immutable root retains an off-fiber payload to exercise the
	// selected transaction's closing step when R1 grows to whole. X already
	// exceeds the fresh R1 on its on-fiber value: this is the real recurrence
	// case where a prior widening step ran ahead of the newly refolded exact.
	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 12)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 7)
	xSource, _, ok := work.MergeContribution(onValue, offValue)
	if !ok {
		t.Fatal("X source")
	}
	x, ok := work.TransportPointState(toPoint(toRule(xSource)), whole, identity, on)
	if !ok || !x.Support().Equal(on) {
		t.Fatal("X")
	}
	xRHS, ok := work.PointRHSFromPointState(x)
	if !ok {
		t.Fatal("X RHS")
	}

	// R0 is the cached exact from the preceding episode. It is deliberately
	// smaller in both authored support and value than the fresh R1 below.
	r0 := toRHS(toRule(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 4)))

	// Build R1 as the canonical closed E+B result. E is an external point
	// environment and B is a back RuleContribution; both author the whole
	// surface, with B raising E's value on the selected factor.
	eRule := toRule(finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 8))
	bRule := toRule(finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 9))
	r1Rule, _, ok := work.MergeRuleContributions(eRule, bRule)
	if !ok {
		t.Fatal("R1 E+B")
	}
	r1 := toRHS(r1Rule)
	r1Point := toPoint(r1Rule)

	if !work.LessOrEqPointRHS(r0, r1) {
		t.Fatal("cached R0 is not below fresh R1")
	}
	if work.LessOrEqPointStateRHS(x, r1) {
		t.Fatal("current X unexpectedly below fresh R1")
	}

	// Independently fold X+E+B in the canonical fixed order used by a region
	// selected RHS. This is the semantic reference for the recurrence result.
	ePoint := toPoint(eRule)
	canonical, ok := foldPointEnvironment(t, work, xRHS, ePoint)
	if !ok {
		t.Fatal("canonical X+E")
	}
	canonical, ok = work.AddRuleContribution(canonical, bRule)
	if !ok {
		t.Fatal("canonical X+E+B")
	}
	if work.LessOrEqPointRHS(canonical, r1) {
		t.Fatal("canonical X+E+B lost X's already-widened value")
	}
	canonicalPoint, ok := work.PublishPointRHS(canonical)
	if !ok {
		t.Fatal("publish canonical selected")
	}

	next, _, ok := work.MergeSelectedPointState(carrier.Widen, x, r1, r1, widenScope)
	if !ok || !next.Support().Equal(whole) || !work.OwnsPointState(next) {
		t.Fatal("selected Widen publication")
	}
	nextRHS, ok := work.PointRHSFromPointState(next)
	if !ok {
		t.Fatal("published RHS")
	}
	if !work.LessOrEqPointStateRHS(x, nextRHS) {
		t.Fatal("selected Widen lost X upper-bound")
	}
	if work.LessOrEqPointRHS(nextRHS, r1) {
		t.Fatal("selected Widen lost the already-widened X value")
	}
	coverage, ok := work.CoverageChangesPointStates(next, r1Point)
	if !ok || coverage.Count() != 0 || coverage.TargetCount() != 0 {
		t.Fatalf("selected Widen changed exact R1 compact C: rows=%d targets=%d", coverage.Count(), coverage.TargetCount())
	}
	if !work.EqualPointState(next, canonicalPoint) {
		t.Fatal("selected Widen differs from canonical X+E+B selected behavior")
	}

	nextRoot, ok := next.HandleAt(slot)
	if !ok {
		t.Fatal("selected root")
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 12 {
		t.Fatalf("selected on value = %d/%t/%t, want 12/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("selected off value = %d/%t/%t, want 9/true/true", got, present, valid)
	}
}
