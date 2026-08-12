package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// neutralTripwireOperation makes any accidental slot traversal observable. A
// neutral merge must return before this callback is entered in either order.
type neutralTripwireOperation struct {
	*carryOnlyOperation
	calls int
}

func (operation *neutralTripwireOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.carryOnlyOperation == nil || operation.prepared {
		return nil, false
	}
	issuer, ok := NewIssuer()
	if !ok {
		return nil, false
	}
	operation.issuer = issuer
	operation.prepared = true
	return operation, true
}

func (operation *neutralTripwireOperation) NewWork() (SlotWork, bool) {
	if operation == nil || operation.carryOnlyOperation == nil {
		return nil, false
	}
	return &neutralTripwireWork{carryOnlyWork: &carryOnlyWork{issuer: operation.issuer}, operation: operation}, true
}

type neutralTripwireWork struct {
	*carryOnlyWork
	operation *neutralTripwireOperation
}

func (work *neutralTripwireWork) MergeContributionUnder(RootHandle, RootHandle, support.Mask, support.Mask, SlotCoverage, SlotCoverage, *support.Work) (ChangeHandle, bool) {
	if work == nil || work.operation == nil {
		return ChangeHandle{}, false
	}
	work.operation.calls++
	return ChangeHandle{}, false
}

func TestNeutralContributionMergeIdentityBothOrdersWithoutSlotTraversal(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	operation := &neutralTripwireOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), empty)
	if !ok {
		t.Fatal("bottom state")
	}
	presentState, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("present state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	neutral, ok := work.EmptyContribution(state)
	if !ok || !work.neutralContribution(neutral) {
		t.Fatal("bottom was not carrier-issued neutral")
	}
	other, ok := work.EmptyContribution(presentState)
	if !ok || work.neutralContribution(other) {
		t.Fatal("nonfalse support was marked neutral")
	}

	left, changes, ok := work.MergeContribution(neutral, other)
	if !ok || !changes.Empty() || left.seal != other.seal || !sameState(left.state, other.state) || operation.calls != 0 {
		t.Fatal("neutral-left merge traversed or changed the right contribution")
	}
	right, changes, ok := work.MergeContribution(other, neutral)
	if !ok || !changes.Empty() || right.seal != other.seal || !sameState(right.state, other.state) || operation.calls != 0 {
		t.Fatal("neutral-right merge traversed or changed the left contribution")
	}
}

func TestNeutralContributionRejectsDifferentScopeAndForeignWork(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	strict, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("strict scope")
	}
	strictState, ok := NewState(composition, strict, empty)
	if !ok {
		t.Fatal("strict bottom state")
	}
	wholeState, ok := NewState(composition, composition.Scope(), empty)
	if !ok {
		t.Fatal("whole bottom state")
	}
	first, ok := composition.NewWork()
	if !ok {
		t.Fatal("first work")
	}
	defer first.Close()
	neutral, ok := first.EmptyContribution(strictState)
	if !ok || !first.neutralContribution(neutral) {
		t.Fatal("strict bottom was not neutral")
	}
	other, ok := first.EmptyContribution(wholeState)
	if !ok {
		t.Fatal("whole contribution")
	}
	if _, _, merged := first.MergeContribution(neutral, other); merged {
		t.Fatal("different scopes crossed neutral identity fence")
	}

	second, ok := composition.NewWork()
	if !ok {
		t.Fatal("second work")
	}
	defer second.Close()
	foreign, ok := second.EmptyContribution(wholeState)
	if !ok || !second.neutralContribution(foreign) {
		t.Fatal("foreign bottom was not neutral")
	}
	if _, _, merged := first.MergeContribution(neutral, foreign); merged {
		t.Fatal("foreign Work neutral crossed ownership fence")
	}
	if _, _, merged := second.MergeContribution(neutral, foreign); merged {
		t.Fatal("foreign Work neutral was accepted by destination")
	}
}

func TestEmptyContributionAcceptsOnlyCompositionInitialRoots(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	operations := []*carryOnlyOperation{{guards: manager}, {guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operations[0], operations[1]})
	if !ok {
		t.Fatal("composition")
	}
	presentState, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("present state")
	}
	bottomState, ok := NewState(composition, composition.Scope(), empty)
	if !ok {
		t.Fatal("bottom state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	present, ok := work.EmptyContribution(presentState)
	if !ok || work.neutralContribution(present) {
		t.Fatal("InitPresent state received neutral proof")
	}
	changed := contributionWrite(t, work, operations[0], bottomState, shape.Slot(0), 2)
	if _, ok := work.EmptyContribution(changed); ok {
		t.Fatal("noninitial raw State was relabelled as an empty contribution")
	}
}

// neutralCoverageOperation issues one valid Target so this law can construct
// an authored sparse Default row without importing a typed Binding package.
type neutralCoverageOperation struct {
	*carryOnlyOperation
	target Target
}

func (operation *neutralCoverageOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.carryOnlyOperation == nil || operation.prepared {
		return nil, false
	}
	issuer, ok := NewIssuer()
	if !ok {
		return nil, false
	}
	target, ok := issuer.IssueTarget(1, StrongTarget)
	if !ok {
		return nil, false
	}
	operation.issuer = issuer
	operation.target = target
	operation.prepared = true
	return operation, true
}

func (operation *neutralCoverageOperation) DeclaredTarget(target Target) bool {
	return operation != nil && operation.prepared && operation.target.Same(target)
}

func (operation *neutralCoverageOperation) ValidTarget(target Target) bool {
	return operation != nil && operation.issuer.Live() && operation.target.Same(target)
}

func TestNeutralContributionRejectsExplicitDefaultCoverage(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	operation := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), empty)
	if !ok {
		t.Fatal("bottom state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	coverage := contributionCoverage{
		composition: composition,
		slots:       []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}},
	}
	explicit, ok := work.admitContribution(state, coverage)
	if !ok || work.neutralContribution(explicit) {
		t.Fatal("explicit Default coverage received neutral proof")
	}
	if _, ok := work.admitNeutralContribution(state, coverage); ok {
		t.Fatal("neutral admission accepted authored coverage")
	}
	neutral, ok := work.EmptyContribution(state)
	if !ok {
		t.Fatal("neutral baseline")
	}
	result, changes, ok := work.MergeContribution(explicit, neutral)
	if !ok || !changes.Empty() || result.seal != explicit.seal || !sameContributionCoverage(result.coverage, explicit.coverage) {
		t.Fatal("explicit coverage was not retained when neutral was on the right")
	}
}
