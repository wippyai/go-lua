package factbinding

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"testing"
)

// TestMergeTransportedPointContributionMatchesBaseline keeps the fused boundary
// honest against the already-proved two-step route.  The cases cover the
// complete coordinate relation surface used by environment edges: exact
// identity, support filtering, expression substitution, noninjective forget,
// absent authored coverage, explicit Default, and coverage-only publication.
func TestMergeTransportedPointContributionMatchesBaseline(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		assertTransportedMatches(t, composition, binding, fixture, leftState, composition.Scope(), whole, plan, whole, whole, whole, true, 4)
	})

	t.Run("filter", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		on, ok := regions.Literal(1, true)
		if !ok || !regions.Seal() {
			t.Fatal("filter region")
		}
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		assertTransportedMatches(t, composition, binding, fixture, leftState, composition.Scope(), whole, plan, on, whole, whole, true, 4)
	})

	t.Run("expression-substitution", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1, 2})
		regions := support.New(manager)
		a, ok := regions.Literal(1, true)
		if !ok {
			t.Fatal("a")
		}
		b, ok := regions.Literal(2, true)
		if !ok {
			t.Fatal("b")
		}
		notB, ok := regions.Literal(2, false)
		if !ok {
			t.Fatal("not-b")
		}
		sourceCell, ok := regions.And(a, notB)
		if !ok || !regions.Seal() {
			t.Fatal("source cell")
		}
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		scope := composition.Scope()
		bExpr, ok := scope.Expr(b)
		if !ok {
			t.Fatal("b expression")
		}
		aExpr, ok := scope.Expr(a)
		if !ok {
			t.Fatal("a expression")
		}
		builder, ok := composition.NewReindex(scope, scope)
		if !ok || !builder.Set(1, bExpr) || !builder.Set(2, aExpr) {
			t.Fatal("substitution builder")
		}
		plan, ok := builder.Seal()
		if !ok {
			t.Fatal("substitution plan")
		}
		assertTransportedMatches(t, composition, binding, fixture, leftState, scope, whole, plan, whole, whole, sourceCell, true, 4)
	})

	t.Run("forget-noninjective", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		on, ok := regions.Literal(1, true)
		if !ok {
			t.Fatal("on")
		}
		off, ok := regions.Literal(1, false)
		if !ok || !regions.Seal() {
			t.Fatal("off")
		}
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		plan, target := forgetPlan(t, composition, composition.Scope(), 1)
		var leftOK bool
		leftState, leftOK = carrier.NewState(composition, target, whole)
		if !leftOK {
			t.Fatal("target left state")
		}
		contributionPlan := compositionPlan(t, composition)
		work := newWork(t, composition)
		onValue := finishContributionAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 3)
		offValue := finishContributionAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 7)
		right, _, ok := work.MergeContribution(onValue, offValue)
		if !ok {
			t.Fatal("merge source fibers")
		}
		left, ok := work.EmptyContribution(leftState)
		if !ok {
			t.Fatal("left contribution")
		}
		assertTransportedContribution(t, work, composition, left, right, target, plan, whole, whole)
	})

	t.Run("empty-coverage", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, _ := bindingState(t, manager, transportConfig(0), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		contributionPlan := compositionPlan(t, composition)
		work := newWork(t, composition)
		base, ok := work.BeginContribution(contributionPlan, composition.Scope(), nil, whole)
		if !ok {
			t.Fatal("begin empty contribution")
		}
		right, ok := work.FinishContribution(base, nil)
		if !ok {
			t.Fatal("finish empty contribution")
		}
		left, ok := work.EmptyContribution(leftState)
		if !ok {
			t.Fatal("left contribution")
		}
		assertTransportedContribution(t, work, composition, left, right, composition.Scope(), plan, whole, whole)
		_ = binding
	})

	t.Run("explicit-default", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		assertTransportedMatches(t, composition, binding, fixture, leftState, composition.Scope(), whole, plan, whole, whole, whole, true, 7)
	})

	t.Run("coverage-only", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		assertTransportedMatches(t, composition, binding, fixture, leftState, composition.Scope(), whole, plan, whole, whole, whole, true, 0)
	})

	t.Run("cancellation", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		whole := transportWhole(t, manager)
		binding, leftState, _, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		plan, ok := composition.IdentityReindex(composition.Scope())
		if !ok {
			t.Fatal("identity plan")
		}
		contributionPlan := compositionPlan(t, composition)
		work := newWork(t, composition)
		right := finishContributionAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
		left, ok := work.EmptyContribution(leftState)
		if !ok {
			t.Fatal("left contribution")
		}
		if !work.SetCheckpoint(func() bool { return false }) {
			t.Fatal("checkpoint")
		}
		if _, _, merged := work.MergeTransportedPointContribution(left, right, whole, plan, whole); merged {
			t.Fatal("canceled fused transport committed")
		}
	})

	t.Run("post-hidden-root-does-not-revive-after-later-support-expansion", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		on, ok := regions.Literal(1, true)
		if !ok {
			t.Fatal("on")
		}
		notOn, ok := regions.Literal(1, false)
		if !ok || !regions.Seal() {
			t.Fatal("regions")
		}
		whole := transportWhole(t, manager)
		binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		scope := composition.Scope()
		expression, ok := scope.Expr(notOn)
		if !ok {
			t.Fatal("target expression")
		}
		builder, ok := composition.NewReindex(scope, scope)
		if !ok || !builder.Set(1, expression) {
			t.Fatal("complement plan")
		}
		plan, ok := builder.Seal()
		if !ok {
			t.Fatal("complement plan seal")
		}
		contributionPlan := compositionPlan(t, composition)
		supportPlan, ok := composition.SealContribution(0, nil, nil, true)
		if !ok {
			t.Fatal("support expansion plan")
		}
		leftState, ok := carrier.NewState(composition, scope, on)
		if !ok {
			t.Fatal("left state")
		}
		work := newWork(t, composition)
		right := finishContributionAt(t, work, contributionPlan, scope, binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
		left, ok := work.EmptyContribution(leftState)
		if !ok {
			t.Fatal("left contribution")
		}
		supportBase, ok := work.BeginContribution(supportPlan, scope, nil, whole)
		if !ok {
			t.Fatal("begin support expansion")
		}
		supportOnly, ok := work.FinishContributionWithSupport(supportBase, nil, whole)
		if !ok {
			t.Fatal("finish support expansion")
		}
		transported, ok := work.TransportPointContribution(right, whole, plan, on)
		if !ok {
			t.Fatal("baseline transport")
		}
		want, _, ok := work.MergeContribution(left, transported)
		if !ok {
			t.Fatal("baseline merge")
		}
		got, _, ok := work.MergeTransportedPointContribution(left, right, whole, plan, on)
		if !ok {
			t.Fatal("fused merge")
		}
		wantExpanded, _, ok := work.MergeContribution(want, supportOnly)
		if !ok {
			t.Fatal("baseline support expansion")
		}
		gotExpanded, _, ok := work.MergeContribution(got, supportOnly)
		if !ok {
			t.Fatal("fused support expansion")
		}
		if !work.EqualUnder(wantExpanded.State(), gotExpanded.State()) || !wantExpanded.Support().Equal(whole) || !gotExpanded.Support().Equal(whole) {
			wantRoot, _ := wantExpanded.HandleAt(slot)
			gotRoot, _ := gotExpanded.HandleAt(slot)
			wantValue, wantPresent, wantValid := observedExactValue(binding, work, wantRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false })
			gotValue, gotPresent, gotValid := observedExactValue(binding, work, gotRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false })
			t.Fatalf("post-hidden root changed during later support expansion: want=%d/%t/%t got=%d/%t/%t", wantValue, wantPresent, wantValid, gotValue, gotPresent, gotValid)
		}
		root, ok := gotExpanded.HandleAt(slot)
		if !ok {
			t.Fatal("expanded root")
		}
		if value, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || value != 0 {
			t.Fatalf("post-clipped payload revived after support growth = %d/%t/%t, want 0/false/true", value, present, valid)
		}
	})

	t.Run("neutral-left-post-empty-does-not-retain-transported-root", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		whole := regions.True()
		empty := regions.False()
		if !regions.Seal() {
			t.Fatal("regions")
		}
		binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		scope := composition.Scope()
		expression, ok := scope.Expr(empty)
		if !ok {
			t.Fatal("target expression")
		}
		builder, ok := composition.NewReindex(scope, scope)
		if !ok || !builder.Set(1, expression) {
			t.Fatal("complement plan")
		}
		plan, ok := builder.Seal()
		if !ok {
			t.Fatal("complement plan seal")
		}
		contributionPlan := compositionPlan(t, composition)
		supportPlan, ok := composition.SealContribution(0, nil, nil, true)
		if !ok {
			t.Fatal("support expansion plan")
		}
		leftState, ok := carrier.NewState(composition, scope, empty)
		if !ok {
			t.Fatal("neutral left state")
		}
		work := newWork(t, composition)
		right := finishContributionAt(t, work, contributionPlan, scope, binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
		left, ok := work.EmptyContribution(leftState)
		if !ok || !left.Valid() {
			t.Fatal("neutral left contribution")
		}
		supportBase, ok := work.BeginContribution(supportPlan, scope, nil, whole)
		if !ok {
			t.Fatal("begin support expansion")
		}
		supportOnly, ok := work.FinishContributionWithSupport(supportBase, nil, whole)
		if !ok {
			t.Fatal("finish support expansion")
		}
		wantTransported, ok := work.TransportPointContribution(right, whole, plan, empty)
		if !ok {
			t.Fatal("baseline transport")
		}
		want, _, ok := work.MergeContribution(left, wantTransported)
		if !ok {
			t.Fatal("baseline neutral merge")
		}
		got, _, ok := work.MergeTransportedPointContribution(left, right, whole, plan, empty)
		if !ok {
			t.Fatal("fused neutral merge")
		}
		wantExpanded, _, ok := work.MergeContribution(want, supportOnly)
		if !ok {
			t.Fatal("baseline expansion")
		}
		gotExpanded, _, ok := work.MergeContribution(got, supportOnly)
		if !ok || !work.EqualUnder(wantExpanded.State(), gotExpanded.State()) {
			t.Fatal("neutral-left fused/baseline closure mismatch")
		}
		root, ok := gotExpanded.HandleAt(slot)
		if !ok {
			t.Fatal("expanded root")
		}
		if value, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || value != 0 {
			t.Fatalf("empty-post payload revived after support growth = %d/%t/%t, want 0/false/true", value, present, valid)
		}
	})

	t.Run("neutral-left-post-empty-does-not-retain-multiple-slots", func(t *testing.T) {
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		whole := regions.True()
		empty := regions.False()
		if !regions.Seal() {
			t.Fatal("regions")
		}
		fixture0, fixture1 := newTestFixture(1), newTestFixture(1)
		config0, config1 := transportConfig(0), transportConfig(0)
		config0.declare, config1.declare = fixture0.declareAllExact, fixture1.declareAllExact
		binding0, ok := bindTest(config0, manager)
		if !ok {
			t.Fatal("binding 0")
		}
		binding1, ok := bindTest(config1, manager)
		if !ok {
			t.Fatal("binding 1")
		}
		composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding0, binding1})
		if !ok {
			t.Fatal("composition")
		}
		scope := composition.Scope()
		expression, ok := scope.Expr(empty)
		if !ok {
			t.Fatal("target expression")
		}
		builder, ok := composition.NewReindex(scope, scope)
		if !ok || !builder.Set(1, expression) {
			t.Fatal("complement plan")
		}
		plan, ok := builder.Seal()
		if !ok {
			t.Fatal("complement plan seal")
		}
		contributionPlan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
		if !ok {
			t.Fatal("contribution plan")
		}
		supportPlan, ok := composition.SealContribution(0, nil, nil, true)
		if !ok {
			t.Fatal("support expansion plan")
		}
		leftState, ok := carrier.NewState(composition, scope, empty)
		if !ok {
			t.Fatal("neutral left state")
		}
		work := newWork(t, composition)
		base, ok := work.BeginContribution(contributionPlan, scope, nil, whole)
		if !ok {
			t.Fatal("begin contribution")
		}
		stage0 := binding0.Begin(work, base.State())
		if stage0 == nil || !stage0.Write(fixture0.target(t, 0, carrier.StrongTarget), whole, 4) {
			t.Fatal("stage slot 0")
		}
		patch0, ok := stage0.Accept(work)
		if !ok {
			t.Fatal("accept slot 0")
		}
		stage1 := binding1.Begin(work, base.State())
		if stage1 == nil || !stage1.Write(fixture1.target(t, 0, carrier.StrongTarget), whole, 5) {
			t.Fatal("stage slot 1")
		}
		patch1, ok := stage1.Accept(work)
		if !ok {
			t.Fatal("accept slot 1")
		}
		right, ok := work.FinishContribution(base, []carrier.Patch{patch0, patch1})
		if !ok {
			t.Fatal("finish contribution")
		}
		left, ok := work.EmptyContribution(leftState)
		if !ok {
			t.Fatal("neutral left contribution")
		}
		supportBase, ok := work.BeginContribution(supportPlan, scope, nil, whole)
		if !ok {
			t.Fatal("begin support expansion")
		}
		supportOnly, ok := work.FinishContributionWithSupport(supportBase, nil, whole)
		if !ok {
			t.Fatal("finish support expansion")
		}
		wantTransported, ok := work.TransportPointContribution(right, whole, plan, empty)
		if !ok {
			t.Fatal("baseline transport")
		}
		want, _, ok := work.MergeContribution(left, wantTransported)
		if !ok {
			t.Fatal("baseline neutral merge")
		}
		got, _, ok := work.MergeTransportedPointContribution(left, right, whole, plan, empty)
		if !ok {
			t.Fatal("fused neutral merge")
		}
		wantExpanded, _, ok := work.MergeContribution(want, supportOnly)
		if !ok {
			t.Fatal("baseline expansion")
		}
		gotExpanded, _, ok := work.MergeContribution(got, supportOnly)
		if !ok || !work.EqualUnder(wantExpanded.State(), gotExpanded.State()) {
			t.Fatal("multi-slot neutral-left fused/baseline closure mismatch")
		}
		root0, ok := gotExpanded.HandleAt(shape.Slot(0))
		if !ok {
			t.Fatal("expanded root 0")
		}
		root1, ok := gotExpanded.HandleAt(shape.Slot(1))
		if !ok {
			t.Fatal("expanded root 1")
		}
		if value, present, valid := observedExactValue(binding0, work, root0, fixture0.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || value != 0 {
			t.Fatalf("empty-post slot 0 payload revived after support growth = %d/%t/%t, want 0/false/true", value, present, valid)
		}
		if value, present, valid := observedExactValue(binding1, work, root1, fixture1.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || value != 0 {
			t.Fatalf("empty-post slot 1 payload revived after support growth = %d/%t/%t, want 0/false/true", value, present, valid)
		}
	})
}

func TestTransportedPointRHSAdoptsClosedOperandOverSupportBase(t *testing.T) {
	manager := testTransportManager(t, nil)
	whole := transportWhole(t, manager)
	binding, initial, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	plan, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity plan")
	}
	writePlan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil, false)
	if !ok {
		t.Fatal("write contribution plan")
	}
	work := newWork(t, composition)
	base, ok := work.EmptyContribution(initial)
	if !ok {
		t.Fatal("support base")
	}
	right := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 7)
	want, _, ok := work.MergeTransportedPointContribution(base, right, whole, plan, whole)
	if !ok {
		t.Fatal("complete transported merge")
	}
	wantRule, ok := work.AsRuleContribution(want)
	if !ok {
		t.Fatal("complete transported merge rule role")
	}
	wantRHS, ok := work.PointRHSFromRuleContribution(wantRule)
	if !ok {
		t.Fatal("complete transported merge RHS role")
	}
	// The canonical fold transaction is the live RHS assembly authority. A
	// coordinate-identical whole transport of a closed operand over a
	// support-only base must reach the same lifted surface as the complete
	// transported merge while adopting that operand's immutable root.
	rightRule, ok := work.AsRuleContribution(right)
	if !ok {
		t.Fatal("operand rule role")
	}
	rightPoint, ok := work.PointStateFromRuleContribution(rightRule)
	if !ok {
		t.Fatal("operand point role")
	}
	transported, ok := work.TransportPointState(rightPoint, whole, plan, whole)
	if !ok {
		t.Fatal("nominal point transport")
	}
	basePoint, ok := work.EmptyPointState(initial)
	if !ok {
		t.Fatal("support-only point base")
	}
	baseRHS, ok := work.PointRHSFromPointState(basePoint)
	if !ok {
		t.Fatal("support-only RHS base")
	}
	if !work.BeginPointRHSFold(basePoint, baseRHS) || !work.AddPointFoldEnvironment(transported) {
		t.Fatal("transported fold inputs")
	}
	got, _, ok := work.FinishPointRHSFold()
	if !ok || !work.EqualPointRHS(got, wantRHS) {
		t.Fatal("RHS adoption differs from complete transported merge")
	}
	transportedRoot, transportedOK := transported.HandleAt(slot)
	gotRoot, gotOK := got.HandleAt(slot)
	if !transportedOK || !gotOK || transportedRoot != gotRoot {
		t.Fatal("RHS adoption rebuilt an immutable closed root")
	}
	rightRoot, rightOK := right.HandleAt(slot)
	if !rightOK || rightRoot != transportedRoot {
		t.Fatal("coordinate-identical whole transport rebuilt an immutable closed root")
	}
}

func transportConfig(defaultValue uint64) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      1,
		Default:     defaultValue,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool { return left <= right },
	}
}

func testTransportManager(t testing.TB, atoms []guard.Atom) *guard.Manager {
	t.Helper()
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func transportWhole(t testing.TB, manager *guard.Manager) support.Mask {
	t.Helper()
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	return whole
}

func compositionPlan(t testing.TB, composition *carrier.Composition) carrier.ContributionPlan {
	t.Helper()
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("contribution plan")
	}
	return plan
}

func assertTransportedMatches(t testing.TB, composition *carrier.Composition, binding *Binding[uint64, uint64], fixture testFixture, leftState carrier.State, leftScope carrier.Scope, leftSupport support.Mask, plan carrier.ReindexPlan, pre, post, sourceSupport support.Mask, write bool, value uint64) {
	t.Helper()
	contributionPlan := compositionPlan(t, composition)
	work := newWork(t, composition)
	var right carrier.Contribution
	var ok bool
	if write {
		right = finishContributionAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), sourceSupport, value)
	} else {
		base, began := work.BeginContribution(contributionPlan, composition.Scope(), nil, sourceSupport)
		if !began {
			t.Fatal("begin contribution")
		}
		right, ok = work.FinishContribution(base, nil)
		if !ok {
			t.Fatal("finish contribution")
		}
	}
	left, ok := work.EmptyContribution(leftState)
	if !ok || !left.Scope().Same(leftScope) || !left.Support().SameHandle(leftSupport) {
		t.Fatal("left contribution")
	}
	assertTransportedContribution(t, work, composition, left, right, leftScope, plan, pre, post)
}

func assertTransportedContribution(t testing.TB, work *carrier.Work, composition *carrier.Composition, left, right carrier.Contribution, target carrier.Scope, plan carrier.ReindexPlan, pre, post support.Mask) {
	t.Helper()
	transported, ok := work.TransportPointContribution(right, pre, plan, post)
	if !ok {
		t.Fatal("baseline transport")
	}
	want, wantChanges, ok := work.MergeContribution(left, transported)
	if !ok {
		t.Fatal("baseline merge")
	}
	got, gotChanges, ok := work.MergeTransportedPointContribution(left, right, pre, plan, post)
	if !ok {
		t.Fatal("fused merge")
	}
	if !got.Scope().Same(target) || !want.Scope().Same(target) || !got.Support().Equal(want.Support()) {
		t.Fatal("fused/baseline support or target scope differs")
	}
	if !work.EqualUnder(got.State(), want.State()) || !work.EqualPointState(closedPointOf(t, work, got), closedPointOf(t, work, want)) {
		t.Fatal("fused/baseline semantic or coverage result differs")
	}
	assertTransportChangeSetsEqual(t, gotChanges, wantChanges)
	if !composition.OwnsChangeSet(gotChanges) || !composition.OwnsChangeSet(wantChanges) {
		t.Fatal("change-set ownership")
	}
}

func assertTransportChangeSetsEqual(t testing.TB, got, want carrier.ChangeSet) {
	t.Helper()
	if !got.Added().Equal(want.Added()) || !got.Removed().Equal(want.Removed()) || got.FactorCount() != want.FactorCount() || got.Count() != want.Count() {
		t.Fatalf("change-set shape differs: got added=%v removed=%v factors=%d rows=%d, want added=%v removed=%v factors=%d rows=%d", got.Added(), got.Removed(), got.FactorCount(), got.Count(), want.Added(), want.Removed(), want.FactorCount(), want.Count())
	}
	for index := 0; index < got.FactorCount(); index++ {
		left, leftOK := got.FactorAt(index)
		right, rightOK := want.FactorAt(index)
		if !leftOK || !rightOK || left.Slot() != right.Slot() || !left.Region().Equal(right.Region()) {
			t.Fatalf("factor change %d differs", index)
		}
	}
	for index := 0; index < got.Count(); index++ {
		left, leftOK := got.At(index)
		right, rightOK := want.At(index)
		if !leftOK || !rightOK || left.Unit() != right.Unit() || !left.Region().Equal(right.Region()) {
			t.Fatalf("unit change %d differs", index)
		}
	}
}
