package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
)

func TestEnvironmentBaselinePreservesUnrelatedFactorSlots(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 2)
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false, true)
	if !ok {
		t.Fatal("environment plan")
	}
	work := mustWork(t, composition)
	leftPatch := contributionPatch(t, work, operations[0], initial, 0, 2)
	rightPatch := contributionPatch(t, work, operations[1], initial, 1, 2)
	environmentState, _, ok := work.Commit(initial, []Patch{leftPatch, rightPatch})
	if !ok {
		t.Fatal("environment state")
	}
	environment, ok := work.EmptyContribution(environmentState)
	if !ok {
		t.Fatal("environment contribution")
	}
	base, ok := work.BeginContribution(plan, composition.Scope(), nil, whole, environment)
	if !ok {
		t.Fatal("environment base")
	}
	patch := contributionPatch(t, work, operations[0], base.State(), 0, 2)
	next, ok := work.FinishContribution(base, []Patch{patch})
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
	plan, ok := composition.SealContribution(0, nil, nil, false, true)
	if !ok {
		t.Fatal("environment plan")
	}
	work := mustWork(t, composition)
	if _, began := work.BeginContribution(plan, composition.Scope(), nil, whole); began {
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
