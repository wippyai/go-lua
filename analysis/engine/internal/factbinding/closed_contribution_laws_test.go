package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// finishContributionWithPremiseAt separates a rule's outer support from its
// authored row.  That distinction is required by the closed-contribution
// laws: an un-authored reachable cell is Absent, rather than Present(Default).
func finishContributionWithPremiseAt(t testing.TB, work *carrier.Work, plan carrier.ContributionPlan, scope carrier.Scope, binding *Binding[uint64, uint64], target carrier.Target, premise, authored support.Mask, value uint64) carrier.Contribution {
	t.Helper()
	base, ok := work.BeginContribution(plan, scope, nil, premise)
	if !ok {
		t.Fatal("begin contribution")
	}
	patch := binding.Begin(work, base.State())
	if patch == nil || !patch.Write(target, authored, value) {
		t.Fatal("write contribution")
	}
	accepted, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept contribution")
	}
	result, ok := work.FinishContribution(base, []carrier.Patch{accepted})
	if !ok || !result.Valid() {
		t.Fatal("finish contribution")
	}
	return result
}

func TestRuleContributionTransportKeepsAbsentOutOfNoninjectiveDefaultJoin(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}
	binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	plan, targetScope := forgetPlan(t, composition, composition.Scope(), 1)
	contributionPlan := compositionPlan(t, composition)
	work := newWork(t, composition)

	// The support reaches both source fibers, but only `on` is authored.  A
	// raw PointState transport must totalize the other source fiber to Default
	// (7); a RuleContribution transport must keep it Absent.
	source := finishContributionWithPremiseAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 4)
	rule, ok := work.TransportRuleContribution(source, whole, plan, whole)
	if !ok || !rule.Scope().Same(targetScope) {
		t.Fatal("lifted rule transport")
	}
	ruleRoot, ok := rule.HandleAt(slot)
	if !ok {
		t.Fatal("rule root")
	}
	if got, present, valid := observedExactValue(binding, work, ruleRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("lifted noninjective transport = %d/%t/%t, want 4/true/true", got, present, valid)
	}

	point, ok := work.TransportPointContribution(source, whole, plan, whole)
	if !ok || !point.Scope().Same(targetScope) {
		t.Fatal("total Point transport")
	}
	pointRoot, ok := point.HandleAt(slot)
	if !ok {
		t.Fatal("point root")
	}
	if got, present, valid := observedExactValue(binding, work, pointRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("total Point transport = %d/%t/%t, want 7/false/true", got, present, valid)
	}
}

func TestRuleContributionTransportPreservesExplicitDefaultDistinctFromAbsent(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}
	binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(7), whole)
	plan, targetScope := forgetPlan(t, composition, composition.Scope(), 1)
	contributionPlan := compositionPlan(t, composition)
	work := newWork(t, composition)

	explicitDefault := finishContributionWithPremiseAt(t, work, contributionPlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 7)
	transported, ok := work.TransportRuleContribution(explicitDefault, whole, plan, whole)
	if !ok || !transported.Scope().Same(targetScope) {
		t.Fatal("transport explicit Default")
	}
	lower := finishContributionAt(t, work, contributionPlan, targetScope, binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
	withDefault, _, ok := work.MergeContribution(lower, transported)
	if !ok {
		t.Fatal("fold explicit Default")
	}
	root, ok := withDefault.HandleAt(slot)
	if !ok {
		t.Fatal("default fold root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("transported Present(Default) fold = %d/%t/%t, want 7/false/true", got, present, valid)
	}

	absentState, ok := carrier.NewState(composition, targetScope, whole)
	if !ok {
		t.Fatal("absent target state")
	}
	absent, ok := work.EmptyContribution(absentState)
	if !ok || work.EqualContribution(transported, absent) {
		t.Fatal("Present(Default) collapsed to Absent")
	}
	preserved, _, ok := work.MergeContribution(lower, absent)
	if !ok {
		t.Fatal("fold absent")
	}
	root, ok = preserved.HandleAt(slot)
	if !ok {
		t.Fatal("absent fold root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("Absent fold = %d/%t/%t, want 4/true/true", got, present, valid)
	}
}

func TestCarryFinishClosesNormalAndPrunedSupportBeforeLaterExpansion(t *testing.T) {
	run := func(t *testing.T, pruned bool) {
		t.Helper()
		manager := testTransportManager(t, []guard.Atom{1})
		regions := support.New(manager)
		on, ok := regions.Literal(1, true)
		if !ok {
			t.Fatal("on")
		}
		whole := regions.True()
		if !regions.Seal() {
			t.Fatal("regions")
		}
		binding, _, slot, composition, fixture := bindingState(t, manager, transportConfig(0), whole)
		writePlan := compositionPlan(t, composition)
		normalCarry, ok := composition.SealContribution(2, nil, []carrier.ContributionSource{{Slot: slot, Input: 0}}, false)
		if !ok {
			t.Fatal("normal carry plan")
		}
		prunedCarry, ok := composition.SealContribution(1, nil, []carrier.ContributionSource{{Slot: slot, Input: 0}}, true)
		if !ok {
			t.Fatal("pruned carry plan")
		}
		expansionPlan, ok := composition.SealContribution(0, nil, nil, true)
		if !ok {
			t.Fatal("support expansion plan")
		}
		work := newWork(t, composition)
		source := finishContributionAt(t, work, writePlan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)

		var carried carrier.Contribution
		if pruned {
			base, began := work.BeginContribution(prunedCarry, composition.Scope(), []carrier.Contribution{source}, whole)
			if !began {
				t.Fatal("begin pruned carry")
			}
			carried, ok = work.FinishContributionWithSupport(base, nil, on)
			if !ok {
				t.Fatal("finish pruned carry")
			}
		} else {
			narrowState, made := carrier.NewState(composition, composition.Scope(), on)
			if !made {
				t.Fatal("narrow input state")
			}
			narrow, made := work.EmptyContribution(narrowState)
			if !made {
				t.Fatal("narrow input contribution")
			}
			base, began := work.BeginContribution(normalCarry, composition.Scope(), []carrier.Contribution{source, narrow}, whole)
			if !began {
				t.Fatal("begin normal carry")
			}
			carried, ok = work.FinishContribution(base, nil)
			if !ok {
				t.Fatal("finish normal carry")
			}
		}
		if !carried.Support().Equal(on) {
			t.Fatal("carry did not retain narrowed support")
		}

		expansionBase, began := work.BeginContribution(expansionPlan, composition.Scope(), nil, whole)
		if !began {
			t.Fatal("begin support expansion")
		}
		expansion, ok := work.FinishContributionWithSupport(expansionBase, nil, whole)
		if !ok {
			t.Fatal("finish support expansion")
		}
		expanded, _, ok := work.MergeContribution(carried, expansion)
		if !ok || !expanded.Support().Equal(whole) {
			t.Fatal("later support expansion")
		}
		root, ok := expanded.HandleAt(slot)
		if !ok {
			t.Fatal("expanded root")
		}
		if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
			t.Fatalf("retained carry value = %d/%t/%t, want 4/true/true", got, present, valid)
		}
		if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
			t.Fatalf("pruned carry payload revived = %d/%t/%t, want 0/false/true", got, present, valid)
		}
	}

	t.Run("normal Finish", func(t *testing.T) { run(t, false) })
	t.Run("FinishWithSupport", func(t *testing.T) { run(t, true) })
}

func TestSelectedContributionWidenPreservesPresenceAndClosesRootToExactCoverage(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("regions")
	}
	binding, _, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	plan := compositionPlan(t, composition)
	selected, ok := composition.SealWidening([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("widen selection")
	}
	work := newWork(t, composition)

	// A selected Widen may raise values, but cannot convert historical
	// Present cells into Absent. This is a C descent even though the raw State
	// support is unchanged, so it must fail before recurrence closure.
	illegalCurrent := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 9)
	illegalExact := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 4)
	if next, _, accepted := work.MergeSelectedContribution(carrier.Widen, illegalCurrent, illegalCurrent, illegalExact, selected); accepted || next.Valid() {
		t.Fatal("selected Widen accepted authored-presence descent")
	}

	// selectedRight must preserve exact authored presence too: a value-only
	// Widen operand may rise, but it cannot add an off-C Present cell.
	illegalSelected := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 9)
	legalCurrent := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 5)
	if next, _, accepted := work.MergeSelectedContribution(carrier.Widen, legalCurrent, illegalSelected, illegalExact, selected); accepted || next.Valid() {
		t.Fatal("selected Widen accepted selected authored-presence descent")
	}

	// exact must also be below selectedRight. Widen only upper-bounds current
	// and selectedRight, so accepting this lower selected operand could publish
	// a result that is not an exact-RHS postfix.
	lowerSelected := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 3)
	if next, _, accepted := work.MergeSelectedContribution(carrier.Widen, legalCurrent, lowerSelected, illegalExact, selected); accepted || next.Valid() {
		t.Fatal("selected Widen accepted exact-not-below-selected operand")
	}

	// Every legal input is already closed to the same exact authored surface:
	// C(current) = C(selected) = C(exact) = on, and exact <= selected.
	current := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 5)
	selectedRight := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 9)
	exact := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 4)
	result, _, ok := work.MergeSelectedContribution(carrier.Widen, current, selectedRight, exact, selected)
	if !ok || !result.Support().Equal(whole) {
		t.Fatal("selected recurrence")
	}
	root, ok := result.HandleAt(slot)
	if !ok {
		t.Fatal("selected root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 9 {
		t.Fatalf("selected exact fiber = %d/%t/%t, want 9/true/true", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("selected recurrence retained out-of-exact payload = %d/%t/%t, want 0/false/true", got, present, valid)
	}
}

func TestSelectedContributionNarrowRequiresExactSelectedOperand(t *testing.T) {
	manager := testTransportManager(t, []guard.Atom{1})
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
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
		t.Fatal("narrow selection")
	}
	work := newWork(t, composition)
	current := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 9)
	exact := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
	foreignSelected := finishContributionWithPremiseAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, on, 5)
	if next, _, accepted := work.MergeSelectedContribution(carrier.Narrow, current, foreignSelected, exact, selected); accepted || next.Valid() {
		t.Fatal("selected Narrow accepted foreign selected RHS")
	}
	result, _, ok := work.MergeSelectedContribution(carrier.Narrow, current, exact, exact, selected)
	if !ok {
		t.Fatal("canonical selected Narrow")
	}
	root, ok := result.HandleAt(slot)
	if !ok {
		t.Fatal("narrow root")
	}
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("canonical Narrow result = %d/%t/%t, want 4/true/true", got, present, valid)
	}
}
