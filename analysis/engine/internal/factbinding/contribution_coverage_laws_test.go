package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestContributionCoverageDistinguishesDefaultNoCandidateKeysAndGuards(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, onOK := regions.Literal(1, true)
	off, offOK := regions.Literal(1, false)
	whole := regions.True()
	if !onOK || !offOK || !regions.Seal() {
		t.Fatal("coverage regions")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, testAlgebraInput[uint64, uint64]{
		KeyEnd:      2,
		Default:     7,
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
	}, whole)
	plan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil, false)
	if !ok {
		t.Fatal("coverage contribution plan")
	}
	carryPlan, ok := composition.SealContribution(1, nil, []carrier.ContributionSource{{Slot: slot, Input: 0}}, false)
	if !ok {
		t.Fatal("carry contribution plan")
	}
	supportPlan, ok := composition.SealContribution(1, nil, nil, true)
	if !ok {
		t.Fatal("support-only contribution plan")
	}
	work := newWork(t, composition)
	base, ok := work.EmptyContribution(initial)
	if !ok {
		t.Fatal("empty point contribution")
	}

	value := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 4)
	defaulted := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), whole, 7)
	noCandidateBase, ok := work.BeginContribution(plan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin no-candidate contribution")
	}
	noCandidate, ok := work.FinishContribution(noCandidateBase, nil)
	if !ok {
		t.Fatal("finish no-candidate contribution")
	}

	withValue, _, ok := work.MergeContribution(base, value)
	if !ok {
		t.Fatal("install value contribution")
	}
	withDefault, defaultChanges, ok := work.MergeContribution(withValue, defaulted)
	if !ok || defaultChanges.FactorCount() != 1 {
		t.Fatal("explicit Default did not participate in Join")
	}
	root, _ := withDefault.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("explicit Default fold = %d/%t/%t, want 7/false/true", got, present, valid)
	}
	preserved, noChanges, ok := work.MergeContribution(withValue, noCandidate)
	if !ok || !noChanges.Empty() {
		t.Fatal("NoCandidate fabricated a semantic contribution")
	}
	root, _ = preserved.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("NoCandidate fold = %d/%t/%t, want 4/true/true", got, present, valid)
	}

	carryBase, ok := work.BeginContribution(carryPlan, composition.Scope(), []carrier.Contribution{defaulted}, whole)
	if !ok {
		t.Fatal("begin exact carry")
	}
	carriedDefault, ok := work.FinishContribution(carryBase, nil)
	if !ok {
		t.Fatal("finish exact carry")
	}
	carriedFold, carriedChanges, ok := work.MergeContribution(withValue, carriedDefault)
	if !ok || carriedChanges.FactorCount() != 1 {
		t.Fatal("carry lost exact Default authorship")
	}
	root, _ = carriedFold.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || present || got != 7 {
		t.Fatalf("carried Default fold = %d/%t/%t, want 7/false/true", got, present, valid)
	}

	supportBase, ok := work.BeginContribution(supportPlan, composition.Scope(), []carrier.Contribution{defaulted}, whole)
	if !ok {
		t.Fatal("begin support-only contribution")
	}
	supportOnly, ok := work.FinishContributionWithSupport(supportBase, nil, whole)
	if !ok {
		t.Fatal("finish support-only contribution")
	}
	noLeak, supportChanges, ok := work.MergeContribution(withValue, supportOnly)
	if !ok || !supportChanges.Empty() {
		t.Fatal("support-only contribution leaked input authorship")
	}
	root, _ = noLeak.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 4 {
		t.Fatalf("support-only fold = %d/%t/%t, want 4/true/true", got, present, valid)
	}

	replaced, replacementChanges, ok := work.ReplaceContribution(defaulted, noCandidate)
	if !ok || !replacementChanges.Empty() || work.EqualContribution(defaulted, replaced) {
		t.Fatal("coverage deletion was hidden or fabricated a semantic change")
	}
	coverageChanges, ok := work.CoverageChanges(defaulted, replaced)
	if !ok || coverageChanges.Count() != 1 {
		t.Fatal("replacement did not publish exact coverage deletion")
	}

	keyOne := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 1, carrier.StrongTarget), whole, 5)
	distinct, _, ok := work.MergeContribution(withValue, keyOne)
	if !ok {
		t.Fatal("different-key contribution fold")
	}
	distinctRoot, _ := distinct.HandleAt(slot)
	for key, want := range []uint64{4, 5} {
		if got, present, valid := observedExactValue(binding, work, distinctRoot, fixture.unit(t, uint64(key)), whole, func(guard.Atom) bool { return false }); !valid || !present || got != want {
			t.Fatalf("different key %d = %d/%t/%t, want %d/true/true", key, got, present, valid, want)
		}
	}

	onValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), on, 4)
	offValue := finishContributionAt(t, work, plan, composition.Scope(), binding, fixture.target(t, 0, carrier.StrongTarget), off, 5)
	guarded, _, ok := work.MergeContribution(base, onValue)
	if ok {
		guarded, _, ok = work.MergeContribution(guarded, offValue)
	}
	if !ok {
		t.Fatal("disjoint guard contribution fold")
	}
	guardedRoot, _ := guarded.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, guardedRoot, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != 4 {
		t.Fatalf("on guard = %d/%t/%t", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, guardedRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 5 {
		t.Fatalf("off guard = %d/%t/%t", got, present, valid)
	}
}

func finishContributionAt(t testing.TB, work *carrier.Work, plan carrier.ContributionPlan, scope carrier.Scope, binding *Binding[uint64, uint64], target carrier.Target, when support.Mask, value uint64) carrier.Contribution {
	t.Helper()
	base, ok := work.BeginContribution(plan, scope, nil, when)
	if !ok {
		t.Fatal("begin contribution")
	}
	patch := binding.Begin(work, base.State())
	if patch == nil || !patch.Write(target, when, value) {
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

func BenchmarkContributionCoverageTargetLocalFold(b *testing.B) {
	manager, err := guard.New(nil)
	if err != nil {
		b.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		b.Fatal("support")
	}
	binding, initial, slot, composition, fixture := bindingState(b, manager, testAlgebraInput[uint64, uint64]{
		KeyEnd:      32,
		Default:     7,
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
	}, whole)
	plan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil, false)
	if !ok {
		b.Fatal("plan")
	}
	work := newWork(b, composition)
	left, ok := work.EmptyContribution(initial)
	if !ok {
		b.Fatal("left")
	}
	right := finishContributionAt(b, work, plan, composition.Scope(), binding, fixture.target(b, 17, carrier.StrongTarget), whole, 11)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, _, merged := work.MergeContribution(left, right); !merged {
			b.Fatal("merge")
		}
	}
	b.ReportMetric(32, "declared-keys/op")
	b.ReportMetric(1, "covered-targets/op")
}
