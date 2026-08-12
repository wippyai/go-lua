package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestPointStateTransportSharesLatentRootButLiftClosesIt exercises the exact
// role split used by the executor: a point boundary may keep an immutable
// semantic root outside its current support, and a sparse RuleContribution
// can patch the reachable authored fiber without rebuilding that latent
// branch. The later forget route proves the raw point still respects source
// support; the lift and confluence cuts prove the latent value never becomes
// an authored fact again.
func TestPointStateTransportSharesLatentRootButLiftClosesIt(t *testing.T) {
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
	binding, initial, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity plan")
	}
	forget, targetScope := forgetPlan(t, composition, composition.Scope(), 1)
	plan := compositionPlan(t, composition)
	work := newWork(t, composition)

	// The closed source owns distinct values on both source fibers.  Filtering
	// the PointState to `on` may retain the physical `off` branch, but it must
	// never become semantic input to the later forget relation.
	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	sourceLegacy, _, ok := work.MergeContribution(onValue, offValue)
	if !ok {
		t.Fatal("merge source fibers")
	}
	source, ok := work.AsRuleContribution(sourceLegacy)
	if !ok {
		t.Fatal("seal source rule")
	}
	point, ok := work.PointStateFromRuleContribution(source)
	if !ok {
		t.Fatal("publish source point")
	}
	narrow, ok := work.TransportPointState(point, whole, identity, on)
	if !ok || !narrow.Support().Equal(on) {
		t.Fatal("narrow point transport")
	}
	sourceRoot, ok := source.HandleAt(slot)
	if !ok {
		t.Fatal("source root")
	}
	narrowRoot, ok := narrow.HandleAt(slot)
	if !ok || narrowRoot != sourceRoot {
		t.Fatal("coordinate point filter rebuilt root instead of sharing it")
	}

	// Adopt the filtered point zero-copy, then apply a sparse on-only rule.
	// The directional RHS operation must update on while retaining the same
	// key's physically latent off=9 branch outside the current point support.
	rhs, ok := work.PointRHSFromPointState(narrow)
	if !ok {
		t.Fatal("adopt narrow point as RHS")
	}
	ruleLegacy := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 5)
	rule, ok := work.AsRuleContribution(ruleLegacy)
	if !ok {
		t.Fatal("seal sparse RHS rule")
	}
	overlaid, ok := work.OverlayRuleContribution(rhs, rule)
	if !ok {
		t.Fatal("overlay sparse RHS rule")
	}
	published, ok := work.PublishPointRHS(overlaid)
	if !ok {
		t.Fatal("publish RHS")
	}
	overlaidRoot, ok := published.HandleAt(slot)
	if !ok || overlaidRoot == narrowRoot {
		t.Fatal("sparse overlay did not publish an on-fiber update")
	}
	if got, present, valid := observedExactValue(binding, work, overlaidRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 5 {
		t.Fatalf("overlaid on branch = %d/%t/%t, want 5/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, overlaidRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("sparse overlay lost latent off branch = %d/%t/%t, want 9/true/true", got, present, valid)
	}

	// Total PointState transport sees only the current support. The latent
	// off=9 root branch is excluded before forget, so the one target fiber is
	// exactly the updated on=5 semantic value.
	routed, ok := work.TransportPointState(published, whole, forget, whole)
	if !ok || !routed.Scope().Same(targetScope) {
		t.Fatal("later semantic point route")
	}
	routedRoot, ok := routed.HandleAt(slot)
	if !ok {
		t.Fatal("routed root")
	}
	if got, present, valid := observedExactValue(binding, work, routedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 5 {
		t.Fatalf("later point route = %d/%t/%t, want 5/true/true", got, present, valid)
	}

	// The lift is the one physical closing cut.  It removes the off branch
	// from the retained root even though that branch was deliberately shared by
	// the filtered PointState above.
	lifted, ok := work.LiftRuleContribution(published)
	if !ok {
		t.Fatal("lift narrowed point")
	}
	liftedRoot, ok := lifted.HandleAt(slot)
	if !ok || liftedRoot == overlaidRoot {
		t.Fatal("lift did not replace latent root with closed rule root")
	}
	if got, present, valid := observedExactValue(binding, work, liftedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("lifted off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}

	// A second, C-empty environment base grows support. PointRHS confluence
	// must first close both lifted surfaces; it may not raw-join default outside
	// C or expose the off=9 latent branch that only existed in the filtered
	// point root.
	emptyPoint, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("whole empty point")
	}
	emptyRHS, ok := work.PointRHSFromPointState(emptyPoint)
	if !ok {
		t.Fatal("whole empty RHS")
	}
	confluent, ok := work.JoinPointRHS(overlaid, emptyRHS)
	if !ok || !confluent.Support().Equal(whole) {
		t.Fatal("confluence with support growth")
	}
	confluentPoint, ok := work.PublishPointRHS(confluent)
	if !ok {
		t.Fatal("publish confluent RHS")
	}
	confluentRoot, ok := confluentPoint.HandleAt(slot)
	if !ok {
		t.Fatal("confluent root")
	}
	if got, present, valid := observedExactValue(binding, work, confluentRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("confluent off branch revived = %d/%t/%t, want 0/false/true", got, present, valid)
	}
}

// TestPointRHSAbsentBaseDoesNotInjectDefault is the hostile Default=Top
// witness. A C-empty point base has a raw sparse terminal whose total semantic
// default is 7, but that terminal is not a lifted RHS operand. A newly
// authored lower value must install as 4 rather than join with 7. The same
// law applies to two environment bases at confluence.
func TestPointRHSAbsentBaseDoesNotInjectDefault(t *testing.T) {
	manager := testTransportManager(t, nil)
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	plan := compositionPlan(t, composition)
	work := newWork(t, composition)

	emptyPoint, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("empty point")
	}
	emptyRHS, ok := work.PointRHSFromPointState(emptyPoint)
	if !ok {
		t.Fatal("empty RHS")
	}
	lowerLegacy := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
	lower, ok := work.AsRuleContribution(lowerLegacy)
	if !ok {
		t.Fatal("lower rule")
	}
	// The empty, composition-initial RHS is the one lawful zero-copy fold
	// identity. Both environment and rule folds must adopt their operand's
	// immutable root header rather than manufacture an otherwise equivalent
	// closed accumulator.
	lowerRoot, ok := lower.HandleAt(slot)
	if !ok {
		t.Fatal("lower root")
	}
	adoptedRule, ok := work.AddRuleContribution(emptyRHS, lower)
	if !ok || !work.OwnsPointRHS(adoptedRule) {
		t.Fatal("adopt rule into empty RHS")
	}
	adoptedRuleRoot, ok := adoptedRule.HandleAt(slot)
	if !ok || adoptedRuleRoot != lowerRoot {
		t.Fatal("empty RHS rule fold rebuilt root")
	}
	lowerPoint, ok := work.PointStateFromRuleContribution(lower)
	if !ok || !work.OwnsPointState(lowerPoint) {
		t.Fatal("lower point")
	}
	adoptedEnvironment, ok := work.AddPointEnvironment(emptyRHS, lowerPoint)
	if !ok || !work.OwnsPointRHS(adoptedEnvironment) {
		t.Fatal("adopt environment into empty RHS")
	}
	adoptedEnvironmentRoot, ok := adoptedEnvironment.HandleAt(slot)
	if !ok || adoptedEnvironmentRoot != lowerRoot {
		t.Fatal("empty RHS environment fold rebuilt root")
	}

	overlaid, ok := work.OverlayRuleContribution(emptyRHS, lower)
	if !ok {
		t.Fatal("overlay empty base")
	}
	overlaidPoint, ok := work.PublishPointRHS(overlaid)
	if !ok {
		t.Fatal("publish overlay")
	}
	overlaidRoot, ok := overlaidPoint.HandleAt(slot)
	if !ok {
		t.Fatal("overlay root")
	}
	if got, present, valid := observedExactValue(binding, work, overlaidRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("C-empty overlay = %d/%t/%t, want 4/true/true", got, present, valid)
	}

	lowerRHS, ok := work.PointRHSFromRuleContribution(lower)
	if !ok {
		t.Fatal("lower RHS")
	}
	confluent, ok := work.JoinPointRHS(emptyRHS, lowerRHS)
	if !ok {
		t.Fatal("confluence empty/lower")
	}
	confluentPoint, ok := work.PublishPointRHS(confluent)
	if !ok {
		t.Fatal("publish confluence")
	}
	confluentRoot, ok := confluentPoint.HandleAt(slot)
	if !ok {
		t.Fatal("confluence root")
	}
	if got, present, valid := observedExactValue(binding, work, confluentRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("C-empty confluence = %d/%t/%t, want 4/true/true", got, present, valid)
	}
}

// TestPointRHSOverlaySharesUntouchedFactorRoot is the root-identity half of
// the guard-route cut. A filtered two-factor point is adopted as an RHS with
// no root rebuild; a Value-only rule changes that root but leaves Pack's
// immutable root handle intact. Lifting the assembled RHS is the sole later
// physical closing cut and removes Pack's latent off-support branch.
func TestPointRHSOverlaySharesUntouchedFactorRoot(t *testing.T) {
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
	valueFixture, packFixture := newTestFixture(1), newTestFixture(1)
	valueConfig, packConfig := transportConfig(0), transportConfig(0)
	valueConfig.declare, packConfig.declare = valueFixture.declareAllExact, packFixture.declareAllExact
	valueBinding, ok := bindTest(valueConfig, manager)
	if !ok {
		t.Fatal("value binding")
	}
	packBinding, ok := bindTest(packConfig, manager)
	if !ok {
		t.Fatal("pack binding")
	}
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{valueBinding, packBinding})
	if !ok {
		t.Fatal("composition")
	}
	allWrites, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("all-writes plan")
	}
	valueWrite, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("value-only plan")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	base, ok := work.BeginContribution(allWrites, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin source")
	}
	valueStage := valueBinding.Begin(work, base.State())
	if valueStage == nil || !valueStage.Write(valueFixture.target(t, 0, carrier.StrongTarget), on, 4) || !valueStage.Write(valueFixture.target(t, 0, carrier.StrongTarget), off, 9) {
		t.Fatal("stage value")
	}
	valuePatch, ok := valueStage.Accept(work)
	if !ok {
		t.Fatal("accept value")
	}
	packStage := packBinding.Begin(work, base.State())
	if packStage == nil || !packStage.Write(packFixture.target(t, 0, carrier.StrongTarget), whole, 6) {
		t.Fatal("stage pack")
	}
	packPatch, ok := packStage.Accept(work)
	if !ok {
		t.Fatal("accept pack")
	}
	sourceLegacy, ok := work.FinishContribution(base, []carrier.Patch{valuePatch, packPatch})
	if !ok {
		t.Fatal("finish source")
	}
	source, ok := work.AsRuleContribution(sourceLegacy)
	if !ok {
		t.Fatal("seal source")
	}
	point, ok := work.PointStateFromRuleContribution(source)
	if !ok {
		t.Fatal("source point")
	}
	filtered, ok := work.TransportPointState(point, whole, identity, on)
	if !ok {
		t.Fatal("filter point")
	}
	valueBefore, ok := filtered.HandleAt(0)
	if !ok {
		t.Fatal("filtered value root")
	}
	packBefore, ok := filtered.HandleAt(1)
	if !ok {
		t.Fatal("filtered pack root")
	}
	// Factor projection is a role-preserving header operation: it shares the
	// chosen Value root and resets Pack to the composition initial root without
	// closing either root. A later factor edge is responsible for point
	// transport and the explicit lift into the closed rule algebra.
	initialBase, ok := work.BeginContribution(allWrites, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin initial roots")
	}
	initialPack, ok := initialBase.State().HandleAt(1)
	if !ok || !work.AbortContribution(initialBase, nil) {
		t.Fatal("capture initial Pack root")
	}
	projected, ok := work.ProjectPointState(filtered, 0)
	if !ok || !work.OwnsPointState(projected) || !projected.Support().Equal(filtered.Support()) {
		t.Fatal("project point Value")
	}
	projectedValue, ok := projected.HandleAt(0)
	if !ok || projectedValue != valueBefore {
		t.Fatal("projection rebuilt selected Value root")
	}
	projectedPack, ok := projected.HandleAt(1)
	if !ok || projectedPack != initialPack || projectedPack == packBefore {
		t.Fatal("projection retained an unselected Pack root")
	}

	ruleBase, ok := work.BeginContribution(valueWrite, composition.Scope(), nil, on)
	if !ok {
		t.Fatal("begin value rule")
	}
	ruleStage := valueBinding.Begin(work, ruleBase.State())
	if ruleStage == nil || !ruleStage.Write(valueFixture.target(t, 0, carrier.StrongTarget), on, 5) {
		t.Fatal("stage value rule")
	}
	rulePatch, ok := ruleStage.Accept(work)
	if !ok {
		t.Fatal("accept value rule")
	}
	ruleLegacy, ok := work.FinishContribution(ruleBase, []carrier.Patch{rulePatch})
	if !ok {
		t.Fatal("finish value rule")
	}
	rule, ok := work.AsRuleContribution(ruleLegacy)
	if !ok {
		t.Fatal("seal value rule")
	}
	rhs, ok := work.PointRHSFromPointState(filtered)
	if !ok {
		t.Fatal("adopt filtered point")
	}
	overlaid, ok := work.OverlayRuleContribution(rhs, rule)
	if !ok {
		t.Fatal("overlay value rule")
	}
	published, ok := work.PublishPointRHS(overlaid)
	if !ok {
		t.Fatal("publish overlay")
	}
	valueAfter, ok := published.HandleAt(0)
	if !ok || valueAfter == valueBefore {
		t.Fatal("Value root was not updated")
	}
	packAfter, ok := published.HandleAt(1)
	if !ok || packAfter != packBefore {
		t.Fatal("Pack root was rebuilt by a Value-only overlay")
	}

	lifted, ok := work.LiftRuleContribution(published)
	if !ok {
		t.Fatal("lift overlay")
	}
	liftedPack, ok := lifted.HandleAt(1)
	if !ok || liftedPack == packAfter {
		t.Fatal("lift did not close Pack's latent root")
	}
	if got, present, valid := observedExactValue(packBinding, work, liftedPack, packFixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("lifted Pack off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}

}

// TestAddPointEnvironmentContainedSurfaceOverlay proves the hot contained
// environment fold is a directional lifted overlay, not a raw State join.
// An environment may retain a non-Default root outside its support while C is
// empty; it must not inject that value (or Factor Default) into the RHS. An
// authored environment does overlay its C cell, preserves the left latent
// branch physically, and explicit Present(Default) remains a real operand.
func TestAddPointEnvironmentContainedSurfaceOverlay(t *testing.T) {
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
	binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	plan := compositionPlan(t, composition)
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)
	toPoint := func(value carrier.Contribution) carrier.PointState {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("rule")
		}
		point, ok := work.PointStateFromRuleContribution(rule)
		if !ok {
			t.Fatal("point")
		}
		return point
	}

	// The left RHS is a coordinate-filtered PointState. Its root still has
	// off=9 physically, but only on=4 is semantically present under C.
	onLeft := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offLeft := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	leftSource, _, ok := work.MergeContribution(onLeft, offLeft)
	if !ok {
		t.Fatal("left source")
	}
	leftPoint, ok := work.TransportPointState(toPoint(leftSource), whole, identity, on)
	if !ok || !leftPoint.Support().Equal(on) {
		t.Fatal("filtered left point")
	}
	leftRHS, ok := work.PointRHSFromPointState(leftPoint)
	if !ok {
		t.Fatal("left RHS")
	}
	leftRoot, ok := leftRHS.HandleAt(slot)
	if !ok {
		t.Fatal("left root")
	}

	// This environment's support is on, but its only authored off row was
	// clipped by the coordinate boundary. It therefore has C empty while its
	// raw root still physically carries off=9. The contained overlay must be a
	// true no-op—not a total State join that sees a default/value on `on`.
	emptySource := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, off, 9)
	emptyEnvironment, ok := work.TransportPointState(toPoint(emptySource), whole, identity, on)
	if !ok || !emptyEnvironment.Support().Equal(on) {
		t.Fatal("filtered C-empty environment")
	}
	emptyRoot, ok := emptyEnvironment.HandleAt(slot)
	if !ok {
		t.Fatal("empty environment root")
	}
	if got, present, valid := observedExactValue(binding, work, emptyRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("C-empty environment latent branch = %d/%t/%t, want 9/true/true", got, present, valid)
	}
	unchanged, ok := work.AddPointEnvironment(leftRHS, emptyEnvironment)
	if !ok || !work.OwnsPointRHS(unchanged) {
		t.Fatal("contained C-empty environment")
	}
	unchangedRoot, ok := unchanged.HandleAt(slot)
	if !ok || unchangedRoot != leftRoot {
		t.Fatal("C-empty environment rebuilt or injected into left root")
	}
	if got, present, valid := observedExactValue(binding, work, unchangedRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("C-empty environment on value = %d/%t/%t, want 4/true/true", got, present, valid)
	}

	// A contained authored environment overlays exactly its C row. It changes
	// on from 4 to 5 but retains left's off=9 branch physically until lift.
	authoredEnvironment := toPoint(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 5))
	overlaid, ok := work.AddPointEnvironment(unchanged, authoredEnvironment)
	if !ok || !overlaid.Support().Equal(on) {
		t.Fatal("contained authored environment")
	}
	overlaidRoot, ok := overlaid.HandleAt(slot)
	if !ok || overlaidRoot == unchangedRoot {
		t.Fatal("authored environment did not update left root")
	}
	if got, present, valid := observedExactValue(binding, work, overlaidRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 5 {
		t.Fatalf("authored environment on value = %d/%t/%t, want 5/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, overlaidRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("authored environment lost left latent branch = %d/%t/%t, want 9/true/true", got, present, valid)
	}
	overlaidPoint, ok := work.PublishPointRHS(overlaid)
	if !ok {
		t.Fatal("publish overlaid environment")
	}
	lifted, ok := work.LiftRuleContribution(overlaidPoint)
	if !ok {
		t.Fatal("lift overlaid environment")
	}
	liftedRoot, ok := lifted.HandleAt(slot)
	if !ok || liftedRoot == overlaidRoot {
		t.Fatal("lift did not close environment overlay")
	}
	if got, present, valid := observedExactValue(binding, work, liftedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("lifted environment off branch = %d/%t/%t, want 7/false/true", got, present, valid)
	}

	// Explicit Default is Present under C even though its sparse root terminal
	// disappears. Joining it over the lower left value must therefore produce
	// Default (7), not retain 4 as an absent environment would.
	defaultEnvironment := toPoint(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 7))
	defaulted, ok := work.AddPointEnvironment(leftRHS, defaultEnvironment)
	if !ok {
		t.Fatal("contained explicit Default environment")
	}
	defaultRoot, ok := defaulted.HandleAt(slot)
	if !ok {
		t.Fatal("explicit Default root")
	}
	if got, present, valid := observedExactValue(binding, work, defaultRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || present || got != 7 {
		t.Fatalf("explicit Default environment = %d/%t/%t, want 7/false/true", got, present, valid)
	}
}

// TestRuleContributionPointInputsCarryCloseLatentAndDefault exercises the
// executor bridge without letting it close a transported PointState at Begin.
// The carried root retains its coordinate-filtered off branch only inside the
// private base; FinishRuleContribution publishes a physically closed rule.
// The second half proves that C carries explicit Present(Default) even when
// that Default has no sparse payload root.
func TestRuleContributionPointInputsCarryCloseLatentAndDefault(t *testing.T) {
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
	binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	writePlan := compositionPlan(t, composition)
	carryPlan, ok := composition.SealContribution(1, nil, []carrier.ContributionSource{{Slot: slot, Input: 0}}, false)
	if !ok {
		t.Fatal("carry plan")
	}
	expansionPlan, ok := composition.SealContribution(0, nil, nil, false)
	if !ok {
		t.Fatal("expansion plan")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	onLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	sourceLegacy, _, ok := work.MergeContribution(onLegacy, offLegacy)
	if !ok {
		t.Fatal("source")
	}
	source, ok := work.AsRuleContribution(sourceLegacy)
	if !ok {
		t.Fatal("source rule")
	}
	point, ok := work.PointStateFromRuleContribution(source)
	if !ok {
		t.Fatal("source point")
	}
	filtered, ok := work.TransportPointState(point, whole, identity, on)
	if !ok || !filtered.Support().Equal(on) {
		t.Fatal("filter point")
	}
	sourceRoot, ok := source.HandleAt(slot)
	if !ok {
		t.Fatal("source root")
	}
	filteredRoot, ok := filtered.HandleAt(slot)
	if !ok || filteredRoot != sourceRoot {
		t.Fatal("filtered input did not retain its latent root")
	}

	base, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{filtered}, whole)
	if !ok || !work.OwnsRuleContributionBase(base, []carrier.PointState{filtered}) || !work.OwnsRuleContributionStates(base, []carrier.State{filtered.State()}) {
		t.Fatal("begin nominal carried rule")
	}
	carried, ok := work.FinishRuleContribution(base, nil)
	if !ok || !carried.Valid() || !carried.Support().Equal(on) {
		t.Fatal("finish nominal carried rule")
	}
	carriedRoot, ok := carried.HandleAt(slot)
	if !ok || carriedRoot == filteredRoot {
		t.Fatal("rule Finish did not close carried latent root")
	}
	if got, present, valid := observedExactValue(binding, work, carriedRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("carried on branch = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, carriedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("carried off branch = %d/%t/%t, want 7/false/true", got, present, valid)
	}

	expansionBase, ok := work.BeginRuleContribution(expansionPlan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin expansion")
	}
	expansion, ok := work.FinishRuleContribution(expansionBase, nil)
	if !ok {
		t.Fatal("finish expansion")
	}
	expanded, _, ok := work.MergeRuleContributions(carried, expansion)
	if !ok || !expanded.Support().Equal(whole) {
		t.Fatal("expand carried rule")
	}
	expandedRoot, ok := expanded.HandleAt(slot)
	if !ok {
		t.Fatal("expanded root")
	}
	if got, present, valid := observedExactValue(binding, work, expandedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("expanded off branch revived = %d/%t/%t, want 7/false/true", got, present, valid)
	}

	explicitDefaultLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 7)
	explicitDefault, ok := work.AsRuleContribution(explicitDefaultLegacy)
	if !ok {
		t.Fatal("explicit Default rule")
	}
	defaultPoint, ok := work.PointStateFromRuleContribution(explicitDefault)
	if !ok {
		t.Fatal("explicit Default point")
	}
	defaultFiltered, ok := work.TransportPointState(defaultPoint, whole, identity, on)
	if !ok {
		t.Fatal("filter explicit Default point")
	}
	defaultBase, ok := work.BeginRuleContribution(carryPlan, composition.Scope(), []carrier.PointState{defaultFiltered}, whole)
	if !ok {
		t.Fatal("begin explicit Default carry")
	}
	defaultCarried, ok := work.FinishRuleContribution(defaultBase, nil)
	if !ok {
		t.Fatal("finish explicit Default carry")
	}
	lowerLegacy := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	lower, ok := work.AsRuleContribution(lowerLegacy)
	if !ok {
		t.Fatal("lower rule")
	}
	withDefault, _, ok := work.MergeRuleContributions(lower, defaultCarried)
	if !ok {
		t.Fatal("merge carried explicit Default")
	}
	defaultRoot, ok := withDefault.HandleAt(slot)
	if !ok {
		t.Fatal("explicit Default root")
	}
	if got, present, valid := observedExactValue(binding, work, defaultRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || present || got != 7 {
		t.Fatalf("carried Present(Default) = %d/%t/%t, want 7/false/true", got, present, valid)
	}
}

// TestChangedCoordinatePointRHSPreservesLatentRootUntilLift is the hostile
// structural-seminaive law. A filtered PointRHS retains an off-support source
// branch physically while a gap-free ascending on-support publication appends
// only its owner-issued change. The later RuleContribution lift and a support
// growing confluence prove that the retained branch was never a semantic RHS
// operand and cannot revive as an authored fact.
func TestChangedCoordinatePointRHSPreservesLatentRootUntilLift(t *testing.T) {
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
	binding, initial, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
	writePlan := compositionPlan(t, composition)
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok || !identity.CoordinateIdentity() {
		t.Fatal("coordinate identity")
	}
	work := newWork(t, composition)

	previousOn := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offSource := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	previousLegacy, _, ok := work.MergeContribution(previousOn, offSource)
	if !ok {
		t.Fatal("previous source")
	}
	upwardOn := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 5)
	currentLegacy, semantic, ok := work.MergeContribution(previousLegacy, upwardOn)
	if !ok {
		t.Fatal("ascending source publication")
	}
	previousRule, ok := work.AsRuleContribution(previousLegacy)
	if !ok {
		t.Fatal("previous rule")
	}
	currentRule, ok := work.AsRuleContribution(currentLegacy)
	if !ok {
		t.Fatal("current rule")
	}
	previousPoint, ok := work.PointStateFromRuleContribution(previousRule)
	if !ok {
		t.Fatal("previous point")
	}
	currentPoint, ok := work.PointStateFromRuleContribution(currentRule)
	if !ok {
		t.Fatal("current point")
	}
	coverageBase, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("coverage base")
	}
	authoredCoverage, ok := work.CoverageChangesPointStates(coverageBase, currentPoint)
	if !ok {
		t.Fatal("point authored coverage changes")
	}
	wakeCoverage, ok := work.CoverageWakeChangesPointStates(coverageBase, currentPoint)
	if !ok || wakeCoverage.TargetCount() != 0 || authoredCoverage.TargetCount() == 0 || wakeCoverage.Count() != authoredCoverage.Count() {
		t.Fatal("point coverage wake projection")
	}
	for index := 0; index < authoredCoverage.Count(); index++ {
		full, fullOK := authoredCoverage.At(index)
		projection, projectionOK := wakeCoverage.At(index)
		if !fullOK || !projectionOK || full.Slot() != projection.Slot() || !full.Region().Equal(projection.Region()) {
			t.Fatal("point coverage wake changed slot/guard meaning")
		}
	}
	previousFiltered, ok := work.TransportPointState(previousPoint, whole, identity, on)
	if !ok || !previousFiltered.Support().Equal(on) {
		t.Fatal("previous filtered point")
	}
	currentFiltered, ok := work.TransportPointState(currentPoint, whole, identity, on)
	if !ok || !currentFiltered.Support().Equal(on) {
		t.Fatal("current filtered point")
	}
	left, ok := work.PointRHSFromPointState(previousFiltered)
	if !ok {
		t.Fatal("left RHS")
	}
	leftRoot, ok := left.HandleAt(slot)
	if !ok {
		t.Fatal("left root")
	}
	authored, ok := work.CoverageChangesPointStates(previousFiltered, currentFiltered)
	if !ok {
		t.Fatal("point coverage changes")
	}

	// The executor's publication window supplies the full lifted
	// previous<=current proof. This carrier path consumes its exact delta
	// without rerunning that expensive proof or closing the PointRHS root.
	appended, ok := work.MergeChangedCoordinatePointRHS(left, previousFiltered, currentFiltered, []carrier.ChangeSet{semantic}, authored, whole, identity, whole)
	if !ok || !appended.Support().Equal(on) || !work.OwnsPointRHS(appended) {
		t.Fatal("changed-coordinate RHS append")
	}
	appendedPoint, ok := work.PublishPointRHS(appended)
	if !ok || !work.EqualPointState(appendedPoint, currentFiltered) || !work.LessOrEqPointStateRHS(previousFiltered, appended) || !work.LessOrEqPointRHSPoint(left, appendedPoint) {
		t.Fatal("nominal lifted comparison after append")
	}
	appendedRoot, ok := appended.HandleAt(slot)
	if !ok || appendedRoot == leftRoot {
		t.Fatal("ascending on-support append did not replace root")
	}
	if got, present, valid := observedExactValue(binding, work, appendedRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 5 {
		t.Fatalf("appended on branch = %d/%t/%t, want 5/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, appendedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 9 {
		t.Fatalf("append lost physical latent off branch = %d/%t/%t, want 9/true/true", got, present, valid)
	}

	lifted, ok := work.LiftRuleContribution(appendedPoint)
	if !ok {
		t.Fatal("lift appended RHS")
	}
	liftedRoot, ok := lifted.HandleAt(slot)
	if !ok || liftedRoot == appendedRoot {
		t.Fatal("lift did not close appended root")
	}
	if got, present, valid := observedExactValue(binding, work, liftedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("lifted off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}

	// Support growth is not legal on the seminaive append itself. It goes
	// through AddPointEnvironment's closed confluence, where the old physical
	// off branch must remain Absent even though the final point support is
	// whole again.
	emptyPoint, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("whole empty point")
	}
	confluent, ok := work.AddPointEnvironment(appended, emptyPoint)
	if !ok || !confluent.Support().Equal(whole) {
		t.Fatal("support-growing confluence")
	}
	confluentRoot, ok := confluent.HandleAt(slot)
	if !ok {
		t.Fatal("confluent root")
	}
	if got, present, valid := observedExactValue(binding, work, confluentRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("confluent off branch revived = %d/%t/%t, want 0/false/true", got, present, valid)
	}
}

// TestMergeSelectedPointStateWidenKeepsExactPresenceAndClosesLatentRoot
// exercises the nominal recurrence boundary directly. Widen may raise an
// on-support value, but current/selected authored presence must remain within
// exact C. The legal filtered PointState deliberately retains an off-support
// payload physically; recurrence publication must close it before returning a
// PointState so no later support confluence can revive it.
func TestMergeSelectedPointStateWidenKeepsExactPresenceAndClosesLatentRoot(t *testing.T) {
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
	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	plan := compositionPlan(t, composition)
	selected, ok := composition.SealWidening([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("widen scope")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	toPoint := func(value carrier.Contribution) carrier.PointState {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("seal rule")
		}
		point, ok := work.PointStateFromRuleContribution(rule)
		if !ok {
			t.Fatal("publish point")
		}
		return point
	}
	toRHS := func(value carrier.Contribution) carrier.PointRHS {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("seal RHS rule")
		}
		rhs, ok := work.PointRHSFromRuleContribution(rule)
		if !ok {
			t.Fatal("publish RHS")
		}
		return rhs
	}

	// All three operands have whole support here, so the rejection below is an
	// authored-presence descent rather than merely a support-shape mismatch.
	illegalCurrent := toPoint(func() carrier.Contribution {
		onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 5)
		offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
		merged, _, ok := work.MergeContribution(onValue, offValue)
		if !ok {
			t.Fatal("illegal current")
		}
		return merged
	}())
	illegalExact := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 4))
	illegalSelected := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 9))
	if next, _, accepted := work.MergeSelectedPointState(carrier.Widen, illegalCurrent, illegalSelected, illegalExact, selected); accepted || next.Valid() {
		t.Fatal("nominal Widen accepted current presence outside exact C")
	}

	// The legal current is a coordinate-filtered published point: C and
	// support are on, but its immutable root still has the physical off=9
	// branch. Selected Widen raises on from 5 to 9; exact C remains on.
	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 5)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 9)
	source, _, ok := work.MergeContribution(onValue, offValue)
	if !ok {
		t.Fatal("source")
	}
	current, ok := work.TransportPointState(toPoint(source), whole, identity, on)
	if !ok || !current.Support().Equal(on) {
		t.Fatal("filtered current")
	}
	currentRoot, ok := current.HandleAt(slot)
	if !ok {
		t.Fatal("current root")
	}
	exact := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 4))
	selectedRight := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 9))
	next, _, ok := work.MergeSelectedPointState(carrier.Widen, current, selectedRight, exact, selected)
	if !ok || !next.Support().Equal(on) || !work.OwnsPointState(next) {
		t.Fatal("legal nominal Widen")
	}
	if !work.LessOrEqPointRHS(exact, selectedRight) {
		t.Fatal("selected RHS does not upper-bound exact RHS")
	}
	nextRoot, ok := next.HandleAt(slot)
	if !ok || nextRoot == currentRoot {
		t.Fatal("Widen did not publish the closed selected root")
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 9 {
		t.Fatalf("Widen on branch = %d/%t/%t, want 9/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("Widen retained latent off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}
	lifted, ok := work.LiftRuleContribution(next)
	if !ok {
		t.Fatal("lift recurrence output")
	}
	liftedRoot, ok := lifted.HandleAt(slot)
	if !ok || liftedRoot != nextRoot {
		t.Fatal("recurrence output was not physically closed")
	}
	wholeEmpty, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("whole empty point")
	}
	nextRHS, ok := work.PointRHSFromPointState(next)
	if !ok {
		t.Fatal("recurrence RHS")
	}
	confluent, ok := work.AddPointEnvironment(nextRHS, wholeEmpty)
	if !ok || !confluent.Support().Equal(whole) {
		t.Fatal("recurrence support confluence")
	}
	confluentRoot, ok := confluent.HandleAt(slot)
	if !ok {
		t.Fatal("confluent root")
	}
	if got, present, valid := observedExactValue(binding, work, confluentRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("Widen confluence revived off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}
}

// TestMergeSelectedPointStateNarrowRequiresExactOperandAndCloses verifies the
// nominal Narrow fence. A distinct selected RHS is rejected even if its shape
// matches exact RHS, and the sole legal selected operand publishes the exact
// C-only root rather than preserving a filtered PointState's latent branch.
func TestMergeSelectedPointStateNarrowRequiresExactOperandAndCloses(t *testing.T) {
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
	config := lawInput(true)
	config.Narrow = func(_, desired uint64) uint64 { return desired }
	config.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	binding, _, slot, composition, fixture := bindingState(t, manager, config, whole)
	plan := compositionPlan(t, composition)
	selected, ok := composition.SealNarrowing([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow scope")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work := newWork(t, composition)

	toPoint := func(value carrier.Contribution) carrier.PointState {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("seal rule")
		}
		point, ok := work.PointStateFromRuleContribution(rule)
		if !ok {
			t.Fatal("publish point")
		}
		return point
	}
	toRHS := func(value carrier.Contribution) carrier.PointRHS {
		t.Helper()
		rule, ok := work.AsRuleContribution(value)
		if !ok {
			t.Fatal("seal RHS rule")
		}
		rhs, ok := work.PointRHSFromRuleContribution(rule)
		if !ok {
			t.Fatal("publish RHS")
		}
		return rhs
	}

	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 9)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 8)
	source, _, ok := work.MergeContribution(onValue, offValue)
	if !ok {
		t.Fatal("source")
	}
	current, ok := work.TransportPointState(toPoint(source), whole, identity, on)
	if !ok || !current.Support().Equal(on) {
		t.Fatal("filtered current")
	}
	currentRoot, ok := current.HandleAt(slot)
	if !ok {
		t.Fatal("current root")
	}
	exact := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 4))
	foreign := toRHS(finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, on, 5))
	if next, _, accepted := work.MergeSelectedPointState(carrier.Narrow, current, foreign, exact, selected); accepted || next.Valid() {
		t.Fatal("nominal Narrow accepted foreign selected RHS")
	}
	next, _, ok := work.MergeSelectedPointState(carrier.Narrow, current, exact, exact, selected)
	if !ok || !next.Support().Equal(on) || !work.OwnsPointState(next) {
		t.Fatal("canonical nominal Narrow")
	}
	nextRoot, ok := next.HandleAt(slot)
	if !ok || nextRoot == currentRoot {
		t.Fatal("Narrow did not publish closed exact root")
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("Narrow on branch = %d/%t/%t, want 4/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, nextRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("Narrow retained latent off branch = %d/%t/%t, want 0/false/true", got, present, valid)
	}
	lifted, ok := work.LiftRuleContribution(next)
	if !ok {
		t.Fatal("lift Narrow output")
	}
	liftedRoot, ok := lifted.HandleAt(slot)
	if !ok || liftedRoot != nextRoot {
		t.Fatal("Narrow output was not physically closed")
	}
}
