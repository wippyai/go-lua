package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// carryOnlyOperation has one immutable root and emits no unit delta. It is a
// carrier-only witness that a support transition does not allocate or replace
// the dense root vector when every Factor carries its prior root.
type carryOnlyOperation struct {
	guards      *guard.Manager
	issuer      Issuer
	prepared    bool
	failReplace bool
}

func (operation *carryOnlyOperation) Preflight() (SlotOperation, bool) {
	issuer, ok := NewIssuer()
	if operation == nil || operation.prepared || !ok {
		return nil, false
	}
	operation.prepared = true
	operation.issuer = issuer
	return operation, true
}

func (operation *carryOnlyOperation) Guards() *guard.Manager { return operation.guards }

func (operation *carryOnlyOperation) Attach(owner SlotOwner) RootHandle {
	if operation == nil || !operation.prepared {
		panic("invalid carry-only attachment")
	}
	operation.issuer.Attach(owner)
	root, ok := operation.issuer.IssueRoot(1)
	if !ok {
		panic("carry-only root")
	}
	return root
}

func (operation *carryOnlyOperation) ValidRoot(root RootHandle) bool {
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && (id == 1 || id == 2)
}

func (operation *carryOnlyOperation) InitialRootReady() bool {
	return operation != nil && operation.prepared
}

func (*carryOnlyOperation) DeclaredUnit(Unit) bool                       { return false }
func (*carryOnlyOperation) DeclaredTarget(Target) bool                   { return false }
func (*carryOnlyOperation) DeclaredSelector(Selector, SelectorKind) bool { return false }
func (*carryOnlyOperation) TargetNotifications(Target) ([]Unit, bool)    { return nil, false }
func (*carryOnlyOperation) PrepareWidening([]Target) (uint64, bool)      { return 0, false }
func (*carryOnlyOperation) PrepareNarrowing([]Target) (uint64, bool)     { return 0, false }
func (*carryOnlyOperation) DeclaredSelectorTargets(Selector) ([]Target, bool) {
	return nil, false
}
func (*carryOnlyOperation) ValidUnit(Unit) bool                       { return false }
func (*carryOnlyOperation) ValidTarget(Target) bool                   { return false }
func (*carryOnlyOperation) ValidSelector(Selector, SelectorKind) bool { return false }
func (operation *carryOnlyOperation) Supports(kind MergeKind) bool {
	return kind == Join || kind == Widen
}

func (operation *carryOnlyOperation) NewWork() (SlotWork, bool) {
	if operation == nil {
		return nil, false
	}
	return &carryOnlyWork{issuer: operation.issuer, failReplace: operation.failReplace}, true
}

type carryOnlyWork struct {
	issuer      Issuer
	failReplace bool
	reenter     func() bool
}

func (*carryOnlyWork) SetCheckpoint(Checkpoint) bool { return true }

func (carryOnlyWork) EqualUnder(RootHandle, RootHandle, support.Mask) bool    { return true }
func (carryOnlyWork) LessOrEqUnder(RootHandle, RootHandle, support.Mask) bool { return true }

func (carryOnlyWork) BeginObservation() bool { return false }

func (carryOnlyWork) EndObservation() bool { return false }

func (carryOnlyWork) ObserveUnder(RootHandle, Unit, support.Mask, func(ObservationRow) bool) bool {
	return false
}

func (work carryOnlyWork) Merge3Under(_ MergeKind, _ bool, _ uint64, left, _ RootHandle, _ support.Split, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work carryOnlyWork) MergeContributionUnder(left, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) OverlayPointRHSUnder(left, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) LessOrEqContributionUnder(_, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage) (bool, bool) {
	return true, true
}

func (work carryOnlyWork) ContributionClosedUnder(RootHandle, support.Mask, SlotCoverage) bool {
	return true
}

func (work carryOnlyWork) ContributionPresenceIncludedUnder(support.Mask, support.Mask, SlotCoverage, SlotCoverage) bool {
	return true
}

func (work carryOnlyWork) MergeTransportedPointUnder(left, _ RootHandle, _, _, _, _ support.Mask, _ guard.Reindex, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) ReindexContributionUnder(left RootHandle, _, _ support.Mask, _ guard.Reindex, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) ReindexPointContributionUnder(left RootHandle, _, _ support.Mask, _ guard.Reindex, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) CloseContributionUnder(left, input RootHandle, _ support.Split, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, input, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) MergeSelectedUnder(_ MergeKind, _ uint64, left, _, right RootHandle, _, _ support.Split, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, right, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) MergeSelectedContributionUnder(_ MergeKind, _ uint64, left, _, right RootHandle, _, _ support.Split, _, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, right, nil, support.Mask{}, nil, nil, delta)
}

func (work carryOnlyWork) ReindexUnder(left RootHandle, _ support.Mask, _ support.Mask, _ guard.Reindex, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work *carryOnlyWork) ReplaceUnder(left, right RootHandle, _ support.Split, delta *support.Work) (ChangeHandle, bool) {
	if work == nil || work.failReplace || work.reenter != nil && work.reenter() {
		return ChangeHandle{}, false
	}
	return work.issuer.IssueChange(left, right, nil, support.Mask{}, nil, nil, delta)
}

func TestWorkOwnershipBindsExactViewPredecessor(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok || !work.OwnsState(state) {
		t.Fatal("own state")
	}
	view, ok := state.Restrict(whole)
	if !ok || !work.OwnsViewOf(state, view) {
		t.Fatal("own view")
	}

	foreignComposition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("foreign composition")
	}
	foreignState, ok := NewState(foreignComposition, foreignComposition.Scope(), whole)
	if !ok {
		t.Fatal("foreign state")
	}
	foreignView, ok := foreignState.Restrict(whole)
	if !ok || work.OwnsState(foreignState) || work.OwnsViewOf(state, foreignView) {
		t.Fatal("foreign composition crossed ownership boundary")
	}

}

func TestWorkCheckpointProbeIsReusedAndRevoked(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	if work.checkpointProbe == nil || work.checkpointFunc() != nil {
		t.Fatal("checkpoint probe was exposed without an installed evaluator checkpoint")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if work.checkpointFunc() != nil {
			t.Fatal("unexpected checkpoint without evaluator probe")
		}
	}); allocations != 0 {
		t.Fatalf("checkpoint selection allocated without evaluator probe: %v", allocations)
	}
	if !work.SetCheckpoint(func() bool { return true }) {
		t.Fatal("install checkpoint")
	}
	probe := work.checkpointFunc()
	if probe == nil || !probe() {
		t.Fatal("checkpoint probe is not live")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if current := work.checkpointFunc(); current == nil || !current() {
			t.Fatal("reused checkpoint probe is not live")
		}
	}); allocations != 0 {
		t.Fatalf("checkpoint selection allocated on hot path: %v", allocations)
	}
	if !work.Close() {
		t.Fatal("close work")
	}
	if work.checkpointProbe != nil || work.checkpointFunc() != nil || probe() {
		t.Fatal("closed work retained a live checkpoint probe")
	}
}

func TestSupportOnlyMergePreservesRootsAndJoinsSupport(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	leftSupport, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("left support")
	}
	rightSupport, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("right support")
	}
	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	left, ok := NewState(composition, composition.Scope(), leftSupport)
	if !ok {
		t.Fatal("left state")
	}
	right, ok := NewState(composition, composition.Scope(), rightSupport)
	if !ok {
		t.Fatal("right state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	next, changes, ok := work.Merge3Under(Join, left, right, composition.AllMergeScope())
	if !ok || changes.Count() != 0 || support.Empty(changes.Added()) || !support.Empty(changes.Removed()) || !leftSupport.Entails(next.Support()) || !rightSupport.Entails(next.Support()) {
		t.Fatalf("support-only join: ok=%t valid=%t support-valid=%t rows=%d added-empty=%t removed-empty=%t covers-left=%t covers-right=%t", ok, next.Valid(), next.Support().Valid(), changes.Count(), support.Empty(changes.Added()), support.Empty(changes.Removed()), leftSupport.Entails(next.Support()), rightSupport.Entails(next.Support()))
	}
}

func TestReplaceSupportOnlySkipsSlotReplacement(t *testing.T) {
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
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	left, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("left")
	}
	right, ok := NewState(composition, composition.Scope(), empty)
	if !ok {
		t.Fatal("right")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	next, changes, ok := work.Replace(left, right)
	if !ok || !support.Empty(next.Support()) || !changes.Removed().Equal(whole) || !support.Empty(changes.Added()) || changes.Count() != 0 {
		t.Fatalf("support-only replace shape: ok=%t rows=%d", ok, changes.Count())
	}
	leftRoot, _ := left.HandleAt(0)
	nextRoot, _ := next.HandleAt(0)
	if leftRoot != nextRoot {
		t.Fatal("support-only replacement copied or altered roots")
	}
}

func TestMergeSelectedUnderEmptyScopeRetractsExactSupport(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("support")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	selection, ok := composition.SealWidening(nil)
	if !ok {
		t.Fatal("empty widening selection")
	}
	current, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("current")
	}
	exact, ok := NewState(composition, composition.Scope(), on)
	if !ok {
		t.Fatal("exact")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	next, changes, ok := work.MergeSelectedUnder(Widen, current, exact, exact, selection)
	if !ok || !next.Support().Equal(on) || !changes.Removed().Equal(off) || !support.Empty(changes.Added()) || changes.Count() != 0 || changes.FactorCount() != 0 {
		t.Fatalf("exact reset: ok=%t support=%t removed=%t added-empty=%t rows=%d factors=%d", ok, next.Support().Equal(on), changes.Removed().Equal(off), support.Empty(changes.Added()), changes.Count(), changes.FactorCount())
	}
}

func TestMergeSelectedUnderEmptyScopeIsFactorFreeExactRight(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	selection, ok := composition.SealWidening(nil)
	if !ok {
		t.Fatal("empty widening selection")
	}
	current, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("current")
	}
	exact, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("exact")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	next, changes, ok := work.MergeSelectedUnder(Widen, current, current, exact, selection)
	if !ok || !work.EqualUnder(next, exact) || !changes.Empty() {
		t.Fatalf("factor-free exact right: ok=%t equal=%t empty=%t", ok, work.EqualUnder(next, exact), changes.Empty())
	}
}

func TestMergeSelectedUnderFailureDropsEarlierPreparedChanges(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first := &carryOnlyOperation{guards: manager}
	second := &carryOnlyOperation{guards: manager, failReplace: true}
	composition, ok := attachTestComposition(t, []FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	selection, ok := composition.SealWidening(nil)
	if !ok {
		t.Fatal("empty widening selection")
	}
	current, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("current")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	if next, _, accepted := work.MergeSelectedUnder(Widen, current, current, current, selection); accepted || next.Valid() {
		t.Fatal("partial selected merge published")
	}
	second.failReplace = false
	work, ok = composition.NewWork()
	if !ok {
		t.Fatal("retry work")
	}
	if next, changes, accepted := work.MergeSelectedUnder(Widen, current, current, current, selection); !accepted || !work.EqualUnder(next, current) || !changes.Empty() {
		t.Fatalf("failed selected merge left partial state: ok=%t equal=%t empty=%t", accepted, work.EqualUnder(next, current), changes.Empty())
	}
}
