package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// adversarialOperation is a carrier-only malicious SlotOperation witness. It
// has no typed payload, but issues one capability so publication visibility can
// be asserted without reaching into a Binding implementation.
type adversarialOperation struct {
	guards       *guard.Manager
	issuer       Issuer
	unit         Unit
	prepared     bool
	initialReady bool
	invalidRoot  bool
	attachments  int
	entered      chan struct{}
	release      <-chan struct{}
}

func newAdversarialOperation(t testing.TB, guards *guard.Manager) *adversarialOperation {
	t.Helper()
	issuer, ok := NewIssuer()
	if !ok {
		t.Fatal("issuer")
	}
	unit, ok := issuer.IssueUnit(ExactUnit, 1, 1)
	if !ok {
		t.Fatal("unit")
	}
	return &adversarialOperation{guards: guards, issuer: issuer, unit: unit, initialReady: true}
}

func (operation *adversarialOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.prepared {
		return nil, false
	}
	operation.prepared = true
	return operation, true
}

func (operation *adversarialOperation) Attach(owner SlotOwner) RootHandle {
	if operation == nil || !operation.prepared {
		panic("invalid adversarial attachment")
	}
	operation.issuer.Attach(owner)
	operation.attachments++
	root, ok := operation.issuer.IssueRoot(1)
	if !ok {
		panic("adversarial root")
	}
	if operation.entered != nil {
		close(operation.entered)
	}
	if operation.release != nil {
		<-operation.release
	}
	if operation.invalidRoot {
		return RootHandle{}
	}
	return root
}

func (operation *adversarialOperation) Guards() *guard.Manager { return operation.guards }

func (operation *adversarialOperation) InitialRootReady() bool {
	return operation != nil && operation.prepared && operation.initialReady
}

func (operation *adversarialOperation) ValidRoot(root RootHandle) bool {
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && (id == 1 || id == 2)
}

func (operation *adversarialOperation) DeclaredUnit(unit Unit) bool {
	return operation != nil && operation.prepared && operation.unit.Same(unit)
}

func (*adversarialOperation) DeclaredTarget(Target) bool                   { return false }
func (*adversarialOperation) DeclaredSelector(Selector, SelectorKind) bool { return false }
func (*adversarialOperation) TargetNotifications(Target) ([]Unit, bool)    { return nil, false }
func (*adversarialOperation) PrepareWidening([]Target) (uint64, bool)      { return 0, false }
func (*adversarialOperation) PrepareNarrowing([]Target) (uint64, bool)     { return 0, false }
func (*adversarialOperation) DeclaredSelectorTargets(Selector) ([]Target, bool) {
	return nil, false
}

func (operation *adversarialOperation) ValidUnit(unit Unit) bool {
	return operation != nil && operation.issuer.Live() && operation.unit.Same(unit)
}

func (*adversarialOperation) ValidTarget(Target) bool                   { return false }
func (*adversarialOperation) ValidSelector(Selector, SelectorKind) bool { return false }
func (*adversarialOperation) Supports(MergeKind) bool                   { return false }
func (*adversarialOperation) NewWork() (SlotWork, bool)                 { return adversarialWork{}, true }

// adversarialWork exists only so carrier publication laws can create a Work
// around a malicious-but-attached SlotOperation. Direct IssueChange tests do
// not call its typed operations.
type adversarialWork struct{}

func (adversarialWork) SetCheckpoint(Checkpoint) bool { return true }

func (adversarialWork) EqualUnder(left, right RootHandle, _ support.Mask) bool { return left == right }
func (adversarialWork) LessOrEqUnder(left, right RootHandle, _ support.Mask) bool {
	return left == right
}
func (adversarialWork) Merge3Under(MergeKind, bool, uint64, RootHandle, RootHandle, support.Split, *support.Work) (ChangeHandle, bool) {
	return ChangeHandle{}, false
}
func (adversarialWork) MergeContributionUnder(RootHandle, RootHandle, support.Mask, support.Mask, SlotCoverage, SlotCoverage, *support.Work) (ChangeHandle, bool) {
	return ChangeHandle{}, false
}
func (adversarialWork) MergeSelectedUnder(MergeKind, uint64, RootHandle, RootHandle, RootHandle, support.Split, support.Split, *support.Work) (ChangeHandle, bool) {
	return ChangeHandle{}, false
}
func (adversarialWork) ReindexUnder(RootHandle, support.Mask, support.Mask, guard.Reindex, *support.Work) (ChangeHandle, bool) {
	return ChangeHandle{}, false
}
func (adversarialWork) ReplaceUnder(RootHandle, RootHandle, support.Split, *support.Work) (ChangeHandle, bool) {
	return ChangeHandle{}, false
}
func (adversarialWork) BeginObservation() bool { return false }
func (adversarialWork) EndObservation() bool   { return false }
func (adversarialWork) ObserveUnder(RootHandle, Unit, support.Mask, func(ObservationRow) bool) bool {
	return false
}

type acceptFailureOperation struct {
	*adversarialOperation
	reject    bool
	publisher *checkpointPublisher
}

func (operation *acceptFailureOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.adversarialOperation == nil || operation.prepared {
		return nil, false
	}
	operation.prepared = true
	return operation, true
}

func (operation *acceptFailureOperation) NewWork() (SlotWork, bool) {
	return acceptFailureWork{operation: operation}, operation != nil
}

func (*acceptFailureOperation) Supports(kind MergeKind) bool { return kind == Join }

type acceptFailureWork struct {
	adversarialWork
	operation *acceptFailureOperation
}

func (work acceptFailureWork) Merge3Under(_ MergeKind, _ bool, _ uint64, left, _ RootHandle, _ support.Split, candidate *support.Work) (ChangeHandle, bool) {
	if work.operation == nil {
		return ChangeHandle{}, false
	}
	after, ok := work.operation.issuer.IssueRoot(2)
	if !ok {
		return ChangeHandle{}, false
	}
	before := left
	if work.operation.reject {
		before = after
	}
	work.operation.publisher = &checkpointPublisher{root: after}
	return work.operation.issuer.IssueChange(before, RootHandle{}, work.operation.publisher, support.Mask{}, nil, nil, candidate)
}

// checkpointPublisher is an intentionally tiny RootPublisher probe. It lets
// the carrier laws place cancellation at the only two meaningful points of an
// ordinary publication attempt: before reservation and immediately after a
// reservation but before the final all-slot cut.
type checkpointPublisher struct {
	root      RootHandle
	onReserve func()
	reserved  int
	published int
	dropped   int
}

func (*checkpointPublisher) Ready() bool { return true }
func (publisher *checkpointPublisher) Reserve() bool {
	publisher.reserved++
	if publisher.onReserve != nil {
		publisher.onReserve()
	}
	return true
}
func (publisher *checkpointPublisher) Publish() RootHandle {
	publisher.published++
	return publisher.root
}
func (publisher *checkpointPublisher) Drop() { publisher.dropped++ }

func checkpointPatch(t testing.TB, work *Work, operation *adversarialOperation, state State, publisher RootPublisher) Patch {
	t.Helper()
	before, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("before")
	}
	whole := state.Support()
	change, ok := operation.issuer.IssueChange(before, RootHandle{}, publisher, whole, nil, nil, nil)
	if !ok {
		t.Fatal("change")
	}
	patch, ok := work.Accept(state, change)
	if !ok {
		t.Fatal("accept")
	}
	return patch
}

func TestCheckpointBeforeCommitDropsPreparedRootWithoutPublication(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := newAdversarialOperation(t, manager)
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
	live := true
	if !work.SetCheckpoint(func() bool { return live }) {
		t.Fatal("checkpoint")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("after")
	}
	publisher := &checkpointPublisher{root: after}
	patch := checkpointPatch(t, work, operation, state, publisher)
	live = false
	if _, _, committed := work.Commit(state, []Patch{patch}); committed {
		t.Fatal("cancelled attempt committed")
	}
	if publisher.reserved != 0 || publisher.published != 0 || publisher.dropped != 1 {
		t.Fatalf("before-commit cleanup = reserve:%d publish:%d drop:%d", publisher.reserved, publisher.published, publisher.dropped)
	}
	before, _ := state.HandleAt(0)
	if now, _ := state.HandleAt(0); now != before {
		t.Fatal("cancelled attempt changed predecessor root")
	}
}

func TestCheckpointAfterReservationDropsEveryPreparedRootBeforeCut(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := newAdversarialOperation(t, manager)
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
	live := true
	if !work.SetCheckpoint(func() bool { return live }) {
		t.Fatal("checkpoint")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("after")
	}
	publisher := &checkpointPublisher{root: after, onReserve: func() { live = false }}
	patch := checkpointPatch(t, work, operation, state, publisher)
	if _, _, committed := work.Commit(state, []Patch{patch}); committed {
		t.Fatal("post-reservation cancellation committed")
	}
	if publisher.reserved != 1 || publisher.published != 0 || publisher.dropped != 1 {
		t.Fatalf("post-reservation cleanup = reserve:%d publish:%d drop:%d", publisher.reserved, publisher.published, publisher.dropped)
	}
	before, _ := state.HandleAt(0)
	if before.id != 1 {
		t.Fatal("post-reservation cancellation exposed a new root")
	}
}

func TestLaterAcceptanceFailureDropsCurrentAndPriorPublishers(t *testing.T) {
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
	first := &acceptFailureOperation{adversarialOperation: newAdversarialOperation(t, manager)}
	second := &acceptFailureOperation{adversarialOperation: newAdversarialOperation(t, manager), reject: true}
	composition, ok := attachTestComposition(t, []FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	left, leftOK := NewState(composition, composition.Scope(), empty)
	right, rightOK := NewState(composition, composition.Scope(), whole)
	work, workOK := composition.NewWork()
	if !leftOK || !rightOK || !workOK {
		t.Fatal("states/work")
	}
	if next, _, merged := work.Merge3Under(Join, left, right, composition.AllMergeScope()); merged || next.Valid() {
		t.Fatal("acceptance failure published a partial merge")
	}
	if first.publisher == nil || second.publisher == nil || first.publisher.published != 0 || second.publisher.published != 0 || first.publisher.dropped != 1 || second.publisher.dropped != 1 {
		t.Fatalf("acceptance cleanup = first:%+v second:%+v", first.publisher, second.publisher)
	}
}

func TestRejectedPublicAcceptRetainsLawfulChangeDropRoute(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := newAdversarialOperation(t, manager)
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, stateOK := NewState(composition, composition.Scope(), whole)
	work, workOK := composition.NewWork()
	if !stateOK || !workOK {
		t.Fatal("state/work")
	}
	wrongBefore, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("wrong predecessor")
	}
	publisher := &checkpointPublisher{root: wrongBefore}
	change, ok := operation.issuer.IssueChange(wrongBefore, RootHandle{}, publisher, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("change")
	}
	if _, accepted := work.Accept(state, change); accepted {
		t.Fatal("public Accept accepted wrong predecessor")
	}
	if publisher.dropped != 0 {
		t.Fatal("rejected public Accept consumed caller-owned handle")
	}
	if !work.DiscardChange(change) || publisher.dropped != 1 || work.DiscardChange(change) {
		t.Fatalf("public change cleanup = drops:%d", publisher.dropped)
	}
}

func TestRejectedInitialRootPreflightAttachesNothing(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	first := newAdversarialOperation(t, manager)
	rejected := newAdversarialOperation(t, manager)
	rejected.initialReady = false
	if prepared, ok := PrepareComposition([]FactorOperation{first, rejected}); ok || prepared != nil {
		t.Fatal("rejected initial root prepared a composition")
	}
	if first.attachments != 0 || rejected.attachments != 0 {
		t.Fatal("failed initial-root preflight attached an operation")
	}
	if _, slotted := first.unit.Slot(); slotted || first.ValidUnit(first.unit) {
		t.Fatal("failed initial-root preflight published an earlier capability")
	}
}

func TestLateInvalidInitialRootCannotPartiallyPublish(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	first := newAdversarialOperation(t, manager)
	invalid := newAdversarialOperation(t, manager)
	invalid.invalidRoot = true
	prepared, ok := PrepareComposition([]FactorOperation{first, invalid})
	if !ok {
		t.Fatal("prepare")
	}
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		_, _ = prepared.Attach()
	}()
	if !panicked || first.attachments != 1 || invalid.attachments != 1 {
		t.Fatal("malformed total attach did not fail at the structural assertion")
	}
	if _, slotted := first.unit.Slot(); slotted || first.ValidUnit(first.unit) {
		t.Fatal("late invalid root partially published an earlier capability")
	}
}

func TestLayoutLatchHidesBoundIssuerUntilEverySlotAttaches(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	first := newAdversarialOperation(t, manager)
	first.entered = make(chan struct{})
	first.release = release
	second := newAdversarialOperation(t, manager)
	prepared, ok := PrepareComposition([]FactorOperation{first, second})
	if !ok {
		t.Fatal("prepare")
	}
	attached := make(chan *Composition, 1)
	go func() {
		composition, attachedOK := prepared.Attach()
		if !attachedOK {
			attached <- nil
			return
		}
		attached <- composition
	}()
	<-first.entered
	if _, slotted := first.unit.Slot(); slotted || first.ValidUnit(first.unit) {
		t.Fatal("first attached slot escaped before the composition latch opened")
	}
	close(release)
	if composition := <-attached; composition == nil {
		t.Fatal("attach")
	}
	if slot, slotted := first.unit.Slot(); !slotted || slot != 0 || !first.ValidUnit(first.unit) {
		t.Fatal("layout latch did not publish first capability after the cut")
	}
}

func TestChangeSetPublishesIssuerDerivedFactorRegionWithoutUnitRows(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := newAdversarialOperation(t, manager)
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
	before, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("before")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("after")
	}
	change, ok := operation.issuer.IssueChange(before, after, nil, whole, nil, nil, nil)
	if !ok {
		t.Fatal("change")
	}
	patch, ok := work.Accept(state, change)
	if !ok {
		t.Fatal("accept")
	}
	next, changes, ok := work.Commit(state, []Patch{patch})
	if !ok || !next.Valid() || changes.Count() != 0 || changes.FactorCount() != 1 {
		t.Fatalf("factor-only change: ok=%t next=%t units=%d factors=%d", ok, next.Valid(), changes.Count(), changes.FactorCount())
	}
	row, ok := changes.FactorAt(0)
	if !ok || row.Slot() != 0 || !row.Region().Equal(whole) {
		t.Fatal("factor row did not use attached issuer slot and exact region")
	}
}

func TestChangeHandleRejectsUnitOutsideFactorRegion(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	factor, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("regions")
	}
	operation := newAdversarialOperation(t, manager)
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	before, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("before")
	}
	after, ok := operation.issuer.IssueRoot(2)
	if !ok {
		t.Fatal("after")
	}
	if _, accepted := operation.issuer.IssueChange(before, after, nil, factor, []Unit{operation.unit}, []support.Mask{whole}, nil); accepted {
		t.Fatal("carrier accepted UnitRegion outside its FactorRegion")
	}
}
