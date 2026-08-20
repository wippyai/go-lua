package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
)

func TestEnvironmentBaselinePreservesUnrelatedFactorSlots(t *testing.T) {
	_, whole, composition, operations, _ := contributionFixture(t, 2)
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, true)
	if !ok {
		t.Fatal("environment plan")
	}
	environmentPlan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil)
	if !ok {
		t.Fatal("environment publication plan")
	}
	work := mustWork(t, composition)
	environment := contributionWrittenPoint(t, work, environmentPlan, composition.Scope(), whole,
		contributionSlotWrite{operation: operations[0], slot: 0, root: 2},
		contributionSlotWrite{operation: operations[1], slot: 1, root: 2})
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), nil, whole, environment)
	if !ok {
		t.Fatal("environment base")
	}
	patch := contributionPatch(t, work, operations[0], base.State(), 0, 2)
	next, ok := work.FinishRuleContribution(base, []Patch{patch})
	if !ok {
		t.Fatal("environment finish")
	}
	left, leftOK := next.HandleAt(0)
	right, rightOK := next.HandleAt(1)
	wantLeft, _ := operations[0].issuer.IssueRoot(2)
	wantRight, _ := operations[1].issuer.IssueRoot(2)
	if !leftOK || !rightOK || left != wantLeft || right != wantRight {
		t.Fatalf("environment roots = (%v,%v), want (%v,%v)", left, right, wantLeft, wantRight)
	}
}

func TestEnvironmentBaselineRequiresSeparateEnvironmentContribution(t *testing.T) {
	_, whole, composition, _, _ := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(0, nil, nil, true)
	if !ok {
		t.Fatal("environment plan")
	}
	work := mustWork(t, composition)
	if _, began := work.BeginRuleContribution(plan, composition.Scope(), nil, whole); began {
		t.Fatal("environment plan accepted missing environment input")
	}
}

func mustWork(t testing.TB, composition *Composition) *Work {
	t.Helper()
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	return work
}
