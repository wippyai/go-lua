package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func forgetPlan(t testing.TB, composition *carrier.Composition, source carrier.Scope, atoms ...guard.Atom) (carrier.ReindexPlan, carrier.Scope) {
	t.Helper()
	target, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := composition.NewReindex(source, target)
	if !ok {
		t.Fatal("reindex builder")
	}
	for _, atom := range atoms {
		if !builder.Forget(atom) {
			t.Fatalf("forget atom %d", atom)
		}
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("reindex plan")
	}
	return plan, target
}

func TestReindexNoninjectiveForgetCollisionJoinsAllReachableFibers(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	plan, target := forgetPlan(t, composition, base.Scope(), 1)
	state := writeStates(t, newWork(t, composition), binding, fixture, base, slot, []factWrite{{when: on, value: 3}, {when: off, value: 7}})
	next, ok := newWork(t, composition).Reindex(state, plan)
	if !ok || !next.Scope().Valid() || next.Scope() == state.Scope() || !next.Support().Equal(whole) {
		t.Fatalf("forget reindex = ok:%t target:%t changed-scope:%t support:%t", ok, next.Scope().Valid(), next.Scope() != state.Scope(), next.Support().Equal(whole))
	}
	root, ok := next.HandleAt(slot)
	if !ok {
		t.Fatal("root")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), next.Support(), func(guard.Atom) bool { return false }); !valid || !present || got != 7 {
		t.Fatalf("forgotten fiber = %d/%t/%t, want 7/true/true", got, present, valid)
	}
	if !next.Scope().Valid() || target != next.Scope() {
		t.Fatal("target scope was not retained on State")
	}
}

func TestReindexExcludesOffSupportFiber(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	plan, _ := forgetPlan(t, composition, base.Scope(), 1)
	full := writeStates(t, newWork(t, composition), binding, fixture, base, slot, []factWrite{{when: on, value: 7}, {when: off, value: 99}})
	view, ok := full.Restrict(on)
	if !ok {
		t.Fatal("on view")
	}
	restricted, _, ok := newWork(t, composition).Transfer(full, view, nil)
	if !ok {
		t.Fatal("restricted state")
	}
	next, ok := newWork(t, composition).Reindex(restricted, plan)
	if !ok || !next.Support().Equal(whole) {
		t.Fatalf("reindex restricted = ok:%t whole:%t", ok, next.Support().Equal(whole))
	}
	root, _ := next.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), next.Support(), func(guard.Atom) bool { return false }); !valid || !present || got != 7 {
		t.Fatalf("off-support branch polluted fiber = %d/%t/%t, want 7/true/true", got, present, valid)
	}
}

func TestReindexSimultaneousSwapAndTargetExprSubstitution(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	a, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("a")
	}
	notA, ok := regions.Literal(1, false)
	if !ok {
		t.Fatal("not a")
	}
	b, ok := regions.Literal(2, true)
	if !ok {
		t.Fatal("b")
	}
	notB, ok := regions.Literal(2, false)
	if !ok {
		t.Fatal("not b")
	}
	on, ok := regions.And(a, notB)
	if !ok {
		t.Fatal("source cell")
	}
	targetCell, ok := regions.And(notA, notB)
	if !ok {
		t.Fatal("target cell")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("seal")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	scope := base.Scope()
	bExpr, ok := scope.Expr(b)
	if !ok {
		t.Fatal("b expr")
	}
	aExpr, ok := scope.Expr(a)
	if !ok {
		t.Fatal("a expr")
	}
	swapBuilder, ok := composition.NewReindex(scope, scope)
	if !ok || !swapBuilder.Set(1, bExpr) || !swapBuilder.Set(2, aExpr) {
		t.Fatal("swap builder")
	}
	swap, ok := swapBuilder.Seal()
	if !ok {
		t.Fatal("swap plan")
	}
	// a := !b and b := a is a general Expr substitution, not a rename. The
	// original a=true,b=false cell therefore reaches target a=false,b=false.
	notBExpr, ok := scope.Expr(notB)
	if !ok {
		t.Fatal("not-b expr")
	}
	exprBuilder, ok := composition.NewReindex(scope, scope)
	if !ok || !exprBuilder.Set(1, notBExpr) || !exprBuilder.Set(2, aExpr) {
		t.Fatal("expr builder")
	}
	exprPlan, ok := exprBuilder.Seal()
	if !ok {
		t.Fatal("expr plan")
	}
	state := writeState(t, newWork(t, composition), binding, fixture, base, slot, on, 13)
	swapped, ok := newWork(t, composition).Reindex(state, swap)
	if !ok {
		t.Fatal("swap")
	}
	root, _ := swapped.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), whole, func(atom guard.Atom) bool { return atom == 2 }); !valid || !present || got != 13 {
		t.Fatalf("swapped target value = %d/%t/%t, want 13/true/true", got, present, valid)
	}

	expressed, ok := newWork(t, composition).Reindex(state, exprPlan)
	if !ok {
		t.Fatal("expr reindex")
	}
	root, _ = expressed.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 13 {
		t.Fatalf("Expr target value = %d/%t/%t, want 13/true/true", got, present, valid)
	}
	if !targetCell.Matches(func(guard.Atom) bool { return false }) {
		t.Fatal("target fixture")
	}
}

func TestReindexCompositionMatchesSequentialStateTransport(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	a, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("a")
	}
	b, ok := regions.Literal(2, true)
	if !ok {
		t.Fatal("b")
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("seal")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(false), whole)
	middle, ok := composition.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("middle scope")
	}
	last, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("last scope")
	}
	firstBuilder, ok := composition.NewReindex(base.Scope(), middle)
	if !ok || !firstBuilder.Identity(1) || !firstBuilder.Identity(2) {
		t.Fatal("first builder")
	}
	first, ok := firstBuilder.Seal()
	if !ok {
		t.Fatal("first plan")
	}
	secondBuilder, ok := composition.NewReindex(middle, last)
	if !ok || !secondBuilder.Forget(1) || !secondBuilder.Forget(2) {
		t.Fatal("second builder")
	}
	second, ok := secondBuilder.Seal()
	if !ok {
		t.Fatal("second plan")
	}
	composed, ok := composition.ComposeReindex(first, second)
	if !ok {
		t.Fatal("composed plan")
	}
	on, ok := support.Intersect(a, b)
	if !ok {
		t.Fatal("source cell")
	}
	state := writeState(t, newWork(t, composition), binding, fixture, base, slot, on, 17)
	sequentialWork := newWork(t, composition)
	middleState, ok := sequentialWork.Reindex(state, first)
	if !ok || middleState.Scope() != middle {
		t.Fatal("first state transport")
	}
	sequential, ok := sequentialWork.Reindex(middleState, second)
	if !ok || sequential.Scope() != last {
		t.Fatal("second state transport")
	}
	direct, ok := newWork(t, composition).Reindex(state, composed)
	if !ok || direct.Scope() != last {
		t.Fatal("direct state transport")
	}
	if !newWork(t, composition).EqualUnder(sequential, direct) {
		t.Fatal("composed State differs from sequential transport")
	}
}

func TestReindexLaterSlotFailureDropsEarlierPreparedRoot(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	on, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on")
	}
	off, ok := regions.Literal(1, false)
	if !ok || !regions.Seal() {
		t.Fatal("off")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	first, fixture := newLawBinding(t, manager, false)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{first, &reindexRejectOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	base, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	plan, _ := forgetPlan(t, composition, base.Scope(), 1)
	state := writeStates(t, newWork(t, composition), first, fixture, base, shape.Slot(0), []factWrite{{when: on, value: 1}, {when: off, value: 2}})
	before, _ := state.HandleAt(shape.Slot(0))
	if _, ok := newWork(t, composition).Reindex(state, plan); ok {
		t.Fatal("later reindex rejection committed earlier root")
	}
	after, _ := state.HandleAt(shape.Slot(0))
	if before != after || !state.Support().SameHandle(whole) {
		t.Fatal("failed reindex changed predecessor")
	}
}

type reindexRejectOperation struct {
	guards   *guard.Manager
	issuer   carrier.Issuer
	prepared bool
}

func (operation *reindexRejectOperation) Preflight() (carrier.SlotOperation, bool) {
	if operation == nil || operation.prepared {
		return nil, false
	}
	issuer, ok := carrier.NewIssuer()
	if !ok {
		return nil, false
	}
	operation.issuer, operation.prepared = issuer, true
	return operation, true
}

func (operation *reindexRejectOperation) Attach(owner carrier.SlotOwner) carrier.RootHandle {
	operation.issuer.Attach(owner)
	root, ok := operation.issuer.IssueRoot(1)
	if !ok {
		panic("reindex rejection root")
	}
	return root
}

func (operation *reindexRejectOperation) Guards() *guard.Manager { return operation.guards }
func (operation *reindexRejectOperation) InitialRootReady() bool {
	return operation != nil && operation.prepared
}
func (operation *reindexRejectOperation) ValidRoot(root carrier.RootHandle) bool {
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && id == 1
}
func (*reindexRejectOperation) DeclaredUnit(carrier.Unit) bool     { return false }
func (*reindexRejectOperation) DeclaredTarget(carrier.Target) bool { return false }
func (*reindexRejectOperation) TargetNotifications(carrier.Target) ([]carrier.Unit, bool) {
	return nil, false
}
func (*reindexRejectOperation) PrepareWidening([]carrier.Target) (uint64, bool) { return 0, false }
func (*reindexRejectOperation) PrepareNarrowing([]carrier.Target) (uint64, bool) {
	return 0, false
}
func (*reindexRejectOperation) ValidUnit(carrier.Unit) bool       { return false }
func (*reindexRejectOperation) ValidTarget(carrier.Target) bool   { return false }
func (*reindexRejectOperation) Supports(carrier.MergeKind) bool   { return false }
func (*reindexRejectOperation) NewWork() (carrier.SlotWork, bool) { return reindexRejectWork{}, true }

type reindexRejectWork struct{}

func (reindexRejectWork) SetCheckpoint(carrier.Checkpoint) bool { return true }

func (reindexRejectWork) EqualUnder(carrier.RootHandle, carrier.RootHandle, support.Mask) bool {
	return true
}
func (reindexRejectWork) LessOrEqUnder(carrier.RootHandle, carrier.RootHandle, support.Mask) bool {
	return true
}
func (reindexRejectWork) Merge3Under(carrier.MergeKind, bool, uint64, carrier.RootHandle, carrier.RootHandle, support.Split, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) MergeContributionUnder(carrier.RootHandle, carrier.RootHandle, support.Mask, support.Mask, carrier.SlotCoverage, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) OverlayPointRHSUnder(carrier.RootHandle, carrier.RootHandle, support.Mask, support.Mask, carrier.SlotCoverage, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) LessOrEqContributionUnder(carrier.RootHandle, carrier.RootHandle, support.Mask, support.Mask, carrier.SlotCoverage, carrier.SlotCoverage) (bool, bool) {
	return false, false
}
func (reindexRejectWork) ContributionClosedUnder(carrier.RootHandle, support.Mask, carrier.SlotCoverage) bool {
	return true
}
func (reindexRejectWork) ContributionPresenceIncludedUnder(support.Mask, support.Mask, carrier.SlotCoverage, carrier.SlotCoverage) bool {
	return true
}
func (reindexRejectWork) MergeTransportedPointUnder(carrier.RootHandle, carrier.RootHandle, support.Mask, support.Mask, support.Mask, support.Mask, guard.Reindex, carrier.SlotCoverage, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) MergeSelectedContributionUnder(carrier.MergeKind, uint64, carrier.RootHandle, carrier.RootHandle, carrier.RootHandle, support.Split, support.Split, carrier.SlotCoverage, carrier.SlotCoverage, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) ReindexContributionUnder(carrier.RootHandle, support.Mask, support.Mask, guard.Reindex, carrier.SlotCoverage, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) ReindexPointContributionUnder(carrier.RootHandle, support.Mask, support.Mask, guard.Reindex, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) CloseContributionUnder(carrier.RootHandle, carrier.RootHandle, support.Split, carrier.SlotCoverage, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) ReindexUnder(carrier.RootHandle, support.Mask, support.Mask, guard.Reindex, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) ReplaceUnder(carrier.RootHandle, carrier.RootHandle, support.Split, *support.Work) (carrier.ChangeHandle, bool) {
	return carrier.ChangeHandle{}, false
}
func (reindexRejectWork) BeginObservation() bool { return false }
func (reindexRejectWork) EndObservation() bool   { return false }
func (reindexRejectWork) ObserveUnder(carrier.RootHandle, carrier.Unit, support.Mask, func(carrier.ObservationRow) bool) bool {
	return false
}
