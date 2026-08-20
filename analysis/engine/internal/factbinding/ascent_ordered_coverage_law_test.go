package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// The lifted order and the ascent-progress relation are different questions
// about the same replacement. LessOrEq asks whether the successor contains the
// predecessor, so authored coverage inclusion is one of its premises: a cell the
// predecessor authored and the successor does not is lifted Absent, and Absent
// is below every present cell. AscentOrdered asks only whether the replacement
// has a defined upper bound, which is the progress a recurrent head already
// relies on when it widens. Carrying LessOrEq's inclusion premise into that
// weaker question turns every coverage movement into a refusal that no caller
// can distinguish from a malformed operand.
//
// These laws hold the two apart over one sealed two-key, two-guard universe.

type ascentCoverageFixture struct {
	work        *carrier.Work
	plan        carrier.ContributionPlan
	binding     *Binding[uint64, uint64]
	composition *carrier.Composition
	fixture     testFixture
	whole       support.Mask
	on          support.Mask
	slot        shape.Slot
}

// contributionUnder authors one write per (key, region) pair while keeping the
// contribution's own outer support at whole, so a law can move authored
// coverage without also moving the support axis the prelude fences.
func (state ascentCoverageFixture) contributionUnder(t testing.TB, writes ...ascentCoverageWrite) carrier.RuleContribution {
	t.Helper()
	base, ok := state.work.BeginContribution(state.plan, state.composition.Scope(), nil, state.whole)
	if !ok {
		t.Fatal("begin ascent-coverage contribution")
	}
	patch := state.binding.Begin(state.work, base.State())
	if patch == nil {
		t.Fatal("begin ascent-coverage patch")
	}
	for _, write := range writes {
		if !patch.Write(state.fixture.target(t, write.key, carrier.StrongTarget), write.region, write.value) {
			t.Fatalf("write ascent-coverage key %d", write.key)
		}
	}
	accepted, ok := patch.Accept(state.work)
	if !ok {
		t.Fatal("accept ascent-coverage patch")
	}
	value, ok := state.work.FinishContribution(base, []carrier.Patch{accepted})
	if !ok || !value.Valid() {
		t.Fatal("finish ascent-coverage contribution")
	}
	rule, ok := state.work.AsRuleContribution(value)
	if !ok {
		t.Fatal("ascent-coverage rule role")
	}
	return rule
}

type ascentCoverageWrite struct {
	key    uint64
	region support.Mask
	value  uint64
}

func newAscentCoverageFixture(t testing.TB, widen func(left, right uint64) uint64) ascentCoverageFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, onOK := regions.Literal(1, true)
	whole := regions.True()
	if !onOK || !regions.Seal() {
		t.Fatal("ascent-coverage regions")
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, testAlgebraInput[uint64, uint64]{
		KeyEnd:      2,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen:    widen,
		LessOrEq: func(left, right uint64) bool { return left <= right },
	}, whole)
	plan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil)
	if !ok {
		t.Fatal("ascent-coverage contribution plan")
	}
	_ = initial
	return ascentCoverageFixture{
		work:        newWork(t, composition),
		plan:        plan,
		binding:     binding,
		composition: composition,
		fixture:     fixture,
		whole:       whole,
		on:          on,
		slot:        slot,
	}
}

func ascentCoverageJoinWiden(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

// TestAscentOrderedAdmitsAShrunkAuthoredRegionBoundedByWiden is the law the
// candidate boundary depends on. A successor that authors the same key over a
// strictly smaller guard region is not above its predecessor, so the order
// refuses it. The ascent relation is a different question: Widen bounds both
// cells, so the replacement is lawful progress and must be answered, not
// refused.
func TestAscentOrderedAdmitsAShrunkAuthoredRegionBoundedByWiden(t *testing.T) {
	state := newAscentCoverageFixture(t, ascentCoverageJoinWiden)
	predecessor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.whole, value: 4})
	successor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.on, value: 4})

	if state.work.LessOrEqRuleContribution(predecessor, successor) {
		t.Fatal("a successor that dropped authorship off the guard is not above its predecessor")
	}
	if !state.work.AscentOrderedRuleContribution(predecessor, successor) {
		t.Fatal("a shrunk authored region with a dominating Widen is lawful ascent progress")
	}
}

// TestAscentOrderedAdmitsARemovedAuthoredKeyBoundedByWiden is the same law on
// the key axis rather than the guard axis.
func TestAscentOrderedAdmitsARemovedAuthoredKeyBoundedByWiden(t *testing.T) {
	state := newAscentCoverageFixture(t, ascentCoverageJoinWiden)
	predecessor := state.contributionUnder(t,
		ascentCoverageWrite{key: 0, region: state.whole, value: 4},
		ascentCoverageWrite{key: 1, region: state.whole, value: 5},
	)
	successor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.whole, value: 4})

	if state.work.LessOrEqRuleContribution(predecessor, successor) {
		t.Fatal("a successor that dropped an authored key is not above its predecessor")
	}
	if !state.work.AscentOrderedRuleContribution(predecessor, successor) {
		t.Fatal("a removed authored key with a dominating Widen is lawful ascent progress")
	}
}

// TestAscentOrderedRefusesAShrunkAuthoredRegionWithNoUpperBound keeps the
// relation from collapsing into a tautology. The same coverage movement over an
// algebra whose Widen does not dominate its predecessor has no upper bound, and
// the ascent relation must still refuse it.
func TestAscentOrderedRefusesAShrunkAuthoredRegionWithNoUpperBound(t *testing.T) {
	state := newAscentCoverageFixture(t, func(_, right uint64) uint64 { return right })
	predecessor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.whole, value: 4})
	successor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.on, value: 4})

	if state.work.LessOrEqRuleContribution(predecessor, successor) {
		t.Fatal("a successor that dropped authorship off the guard is not above its predecessor")
	}
	if state.work.AscentOrderedRuleContribution(predecessor, successor) {
		t.Fatal("a replacement with no dominating Widen is not ascent progress")
	}
}

// TestAscentOrderedKeepsTheSupportAxisFence is the negative half on the outer
// axis: coverage is the authorship relation, support is feasibility, and the
// ascent relation still requires the predecessor's support to be included.
func TestAscentOrderedKeepsTheSupportAxisFence(t *testing.T) {
	state := newAscentCoverageFixture(t, ascentCoverageJoinWiden)
	predecessor := state.contributionUnder(t, ascentCoverageWrite{key: 0, region: state.whole, value: 4})
	narrowBase, ok := state.work.BeginContribution(state.plan, state.composition.Scope(), nil, state.on)
	if !ok {
		t.Fatal("begin narrowed-support contribution")
	}
	narrowed, ok := state.work.FinishContribution(narrowBase, nil)
	if !ok {
		t.Fatal("finish narrowed-support contribution")
	}
	successor, ok := state.work.AsRuleContribution(narrowed)
	if !ok {
		t.Fatal("narrowed-support rule role")
	}
	if state.work.AscentOrderedRuleContribution(predecessor, successor) {
		t.Fatal("ascent progress must not be claimed across a narrowed outer support")
	}
}
