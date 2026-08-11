package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// sparseContributionTripwireOperation keeps the typed slot operation valid,
// but makes an unexpected MergeContributionUnder traversal observable.  The
// carrier fast path must still publish support and coverage through its normal
// cuts; only the typed merge itself is forbidden in the cases below.
type sparseContributionTripwireOperation struct {
	*carryOnlyOperation
	target     Target
	withTarget bool
	calls      int
}

func (operation *sparseContributionTripwireOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.carryOnlyOperation == nil || operation.prepared {
		return nil, false
	}
	issuer, ok := NewIssuer()
	if !ok {
		return nil, false
	}
	if operation.withTarget {
		operation.target, ok = issuer.IssueTarget(1, StrongTarget)
		if !ok {
			return nil, false
		}
	}
	operation.issuer = issuer
	operation.prepared = true
	return operation, true
}

func (operation *sparseContributionTripwireOperation) DeclaredTarget(target Target) bool {
	return operation != nil && operation.withTarget && operation.prepared && operation.target.Same(target)
}

func (operation *sparseContributionTripwireOperation) ValidTarget(target Target) bool {
	return operation != nil && operation.withTarget && operation.issuer.Live() && operation.target.Same(target)
}

func (operation *sparseContributionTripwireOperation) NewWork() (SlotWork, bool) {
	if operation == nil || operation.carryOnlyOperation == nil {
		return nil, false
	}
	return &sparseContributionTripwireWork{
		carryOnlyWork: &carryOnlyWork{issuer: operation.issuer},
		operation:     operation,
	}, true
}

type sparseContributionTripwireWork struct {
	*carryOnlyWork
	operation *sparseContributionTripwireOperation
}

func (work *sparseContributionTripwireWork) MergeContributionUnder(left, right RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	if work == nil || work.operation == nil {
		return ChangeHandle{}, false
	}
	work.operation.calls++
	return work.carryOnlyWork.MergeContributionUnder(left, right, leftSupport, rightSupport, leftCoverage, rightCoverage, delta)
}

func TestMergeContributionRightEmptySlotSkipsTypedTraversalAndRetainsSupportChange(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	leftSupport, leftOK := regions.Literal(1, false)
	rightSupport, rightOK := regions.Literal(1, true)
	whole, wholeOK := support.True(manager)
	if !leftOK || !rightOK || !wholeOK || !regions.Seal() {
		t.Fatal("support regions")
	}
	operation := &sparseContributionTripwireOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	leftState, ok := NewState(composition, composition.Scope(), leftSupport)
	if !ok {
		t.Fatal("left state")
	}
	rightState, ok := NewState(composition, composition.Scope(), rightSupport)
	if !ok {
		t.Fatal("right state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	changedLeft := contributionWrite(t, work, operation.carryOnlyOperation, leftState, shape.Slot(0), 2)
	left, ok := work.EmptyContribution(changedLeft)
	if !ok {
		t.Fatal("left contribution")
	}
	right, ok := work.EmptyContribution(rightState)
	if !ok {
		t.Fatal("right contribution")
	}
	merged, changes, ok := work.MergeContribution(left, right)
	if !ok {
		t.Fatal("merge")
	}
	if operation.calls != 0 {
		t.Fatalf("right-empty slot traversed typed merge %d times", operation.calls)
	}
	if !merged.Support().Equal(whole) || !changes.Added().Equal(rightSupport) || !support.Empty(changes.Removed()) {
		t.Fatal("support union change was not published")
	}
	root, ok := merged.HandleAt(shape.Slot(0))
	if !ok || root != changedLeft.roots[0] {
		t.Fatal("right-empty fold replaced the left root")
	}
}

func TestMergeContributionSameRootKeepsCoverageOnlyWakeWithoutTypedTraversal(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	leftSupport, leftOK := regions.Literal(1, false)
	rightSupport, rightOK := regions.Literal(1, true)
	whole, wholeOK := support.True(manager)
	if !leftOK || !rightOK || !wholeOK || !regions.Seal() {
		t.Fatal("support regions")
	}
	operation := &sparseContributionTripwireOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}, withTarget: true}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	leftState, ok := NewState(composition, composition.Scope(), leftSupport)
	if !ok {
		t.Fatal("left state")
	}
	rightState, ok := NewState(composition, composition.Scope(), rightSupport)
	if !ok {
		t.Fatal("right state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	changedLeft := contributionWrite(t, work, operation.carryOnlyOperation, leftState, shape.Slot(0), 2)
	changedRight := contributionWrite(t, work, operation.carryOnlyOperation, rightState, shape.Slot(0), 2)
	left, ok := work.EmptyContribution(changedLeft)
	if !ok {
		t.Fatal("left contribution")
	}
	coverage := contributionCoverage{
		composition: composition,
		slots:       []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: rightSupport}}}},
	}
	right, ok := work.admitContribution(changedRight, coverage)
	if !ok {
		t.Fatal("right authored contribution")
	}
	merged, changes, ok := work.MergeContribution(left, right)
	if !ok {
		t.Fatal("merge")
	}
	if operation.calls != 0 {
		t.Fatalf("same-root slot traversed typed merge %d times", operation.calls)
	}
	if !merged.Support().Equal(whole) || !changes.Added().Equal(rightSupport) || !support.Empty(changes.Removed()) {
		t.Fatal("same-root support change was not published")
	}
	coverageChanges, ok := work.CoverageChanges(left, merged)
	if !ok || coverageChanges.Count() != 1 {
		t.Fatal("same-root authored coverage was not retained")
	}
	root, ok := merged.HandleAt(shape.Slot(0))
	if !ok || root != changedLeft.roots[0] {
		t.Fatal("same-root fold replaced the left root")
	}
}

func TestMergeContributionForeignScopeStillRejectsSparseFastPath(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	firstOperation := &sparseContributionTripwireOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	secondOperation := &sparseContributionTripwireOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	firstComposition, ok := attachTestComposition(t, []FactorOperation{firstOperation})
	if !ok {
		t.Fatal("first composition")
	}
	secondComposition, ok := attachTestComposition(t, []FactorOperation{secondOperation})
	if !ok {
		t.Fatal("second composition")
	}
	firstState, ok := NewState(firstComposition, firstComposition.Scope(), whole)
	if !ok {
		t.Fatal("first state")
	}
	secondState, ok := NewState(secondComposition, secondComposition.Scope(), whole)
	if !ok {
		t.Fatal("second state")
	}
	firstWork, ok := firstComposition.NewWork()
	if !ok {
		t.Fatal("first work")
	}
	defer firstWork.Close()
	secondWork, ok := secondComposition.NewWork()
	if !ok {
		t.Fatal("second work")
	}
	defer secondWork.Close()
	left, ok := firstWork.EmptyContribution(firstState)
	if !ok {
		t.Fatal("left contribution")
	}
	foreign, ok := secondWork.EmptyContribution(secondState)
	if !ok {
		t.Fatal("foreign contribution")
	}
	if _, _, merged := firstWork.MergeContribution(left, foreign); merged {
		t.Fatal("foreign sparse contribution crossed the owner fence")
	}
}
