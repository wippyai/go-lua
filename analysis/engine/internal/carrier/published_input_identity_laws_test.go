package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestPublishedInputIdentitySeparatesSemanticSupportFromPredecessorHandles
// keeps the input/publication identity law next to its carrier authority. Two
// support works can publish distinct BDD handles for the same formula; that
// must not invalidate an input or Point representation. Replacing the
// immutable root vector remains a publication event.
func TestPublishedInputIdentitySeparatesSemanticSupportFromPredecessorHandles(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	firstWork := support.New(manager)
	firstSupport, firstOK := firstWork.Literal(1, true)
	if !firstOK || !firstWork.Seal() {
		t.Fatal("first support")
	}
	secondWork := support.New(manager)
	secondSupport, secondOK := secondWork.Literal(1, true)
	if !secondOK || !secondWork.Seal() {
		t.Fatal("second support")
	}
	if !firstSupport.Equal(secondSupport) || firstSupport.SameHandle(secondSupport) {
		t.Fatal("support fixture did not produce equal distinct handles")
	}

	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	firstState, ok := NewState(composition, composition.Scope(), firstSupport)
	if !ok {
		t.Fatal("first state")
	}
	secondState, ok := NewState(composition, composition.Scope(), secondSupport)
	if !ok {
		t.Fatal("second state")
	}
	if firstState.Same(secondState) {
		t.Fatal("strict predecessor identity ignored support-handle replacement")
	}

	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	firstPoint, ok := work.EmptyPointState(firstState)
	if !ok {
		t.Fatal("first point")
	}
	secondPoint, ok := work.EmptyPointState(secondState)
	if !ok {
		t.Fatal("second point")
	}
	if !work.ExactSamePointRepresentation(firstPoint, secondPoint) {
		t.Fatal("point version identity changed for equivalent support")
	}
	if !work.EqualPointState(firstPoint, secondPoint) {
		t.Fatal("equivalent support changed point semantics")
	}

	replacement, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("replacement root")
	}
	changedState := firstState
	changedState.roots = append([]RootHandle(nil), firstState.roots...)
	changedState.roots[0] = replacement
	changedContribution, ok := work.admitContribution(changedState, contributionCoverage{composition: composition})
	if !ok {
		t.Fatal("replaced-root contribution")
	}
	if work.ExactSamePointRepresentation(contributionPointOf(t, work, changedContribution), firstPoint) {
		t.Fatal("root-vector replacement did not cross publication identity")
	}
	if work.ExactSamePointRepresentation(firstPoint, secondPoint) && !work.EqualPointState(firstPoint, secondPoint) {
		t.Fatal("exact point identity admitted a semantic change")
	}
}

// TestPointPublicationIdentityIncludesCompactCoverage proves that the shared
// State identity is only the first half of a Point publication identity. A
// compact Target x Guard row changes the exact Point representation and emits
// the structural wake projection even when roots and support are unchanged.
func TestPointPublicationIdentityIncludesCompactCoverage(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
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
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	empty, ok := work.EmptyPointState(state)
	if !ok {
		t.Fatal("empty point")
	}
	covered := contributionCoverage{
		composition: composition,
		slots:       []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}},
	}
	coveredContribution, ok := work.admitContribution(state, covered)
	if !ok {
		t.Fatal("covered contribution")
	}
	coveredRule, ok := work.AsRuleContribution(coveredContribution)
	if !ok {
		t.Fatal("covered rule")
	}
	coveredPoint, ok := work.PointStateFromRuleContribution(coveredRule)
	if !ok {
		t.Fatal("covered point")
	}
	if work.ExactSamePointRepresentation(empty, coveredPoint) {
		t.Fatal("compact coverage change was hidden by State identity")
	}
	wakes, ok := work.CoverageWakeChangesPointStates(empty, coveredPoint)
	if !ok || wakes.Count() == 0 || wakes.TargetCount() != 0 {
		t.Fatalf("compact coverage wake = ok:%t rows:%d targets:%d", ok, wakes.Count(), wakes.TargetCount())
	}
}
