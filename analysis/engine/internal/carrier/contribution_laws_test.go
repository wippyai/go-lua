package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func contributionFixture(t testing.TB, count int) (*guard.Manager, support.Mask, *Composition, []*carryOnlyOperation, State) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	operations := make([]FactorOperation, count)
	values := make([]*carryOnlyOperation, count)
	for index := range values {
		values[index] = &carryOnlyOperation{guards: manager}
		operations[index] = values[index]
	}
	composition, ok := attachTestComposition(t, operations)
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("initial state")
	}
	return manager, whole, composition, values, state
}

func contributionPatch(t testing.TB, work *Work, operation *carryOnlyOperation, predecessor State, slot shape.Slot, rootID uint64) Patch {
	t.Helper()
	before, ok := predecessor.HandleAt(slot)
	if !ok {
		t.Fatal("patch before root")
	}
	after, ok := operation.issuer.IssueRoot(rootID)
	if !ok {
		t.Fatal("patch after root")
	}
	change, ok := operation.issuer.IssueChange(before, after, nil, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("patch change")
	}
	patch, ok := work.Accept(predecessor, change)
	if !ok {
		t.Fatal("accepted patch")
	}
	return patch
}

func contributionWrite(t testing.TB, work *Work, operation *carryOnlyOperation, predecessor State, slot shape.Slot, rootID uint64) State {
	t.Helper()
	patch := contributionPatch(t, work, operation, predecessor, slot, rootID)
	next, _, ok := work.Commit(predecessor, []Patch{patch})
	if !ok {
		t.Fatal("write contribution input")
	}
	return next
}

// contributionPoint pairs one immutable initial State with the empty authored
// surface a Rule input port carries when no producer has published a factor
// yet. It is the same route the executor takes for a neutral point base.
func contributionPoint(t testing.TB, work *Work, state State) PointState {
	t.Helper()
	point, ok := work.EmptyPointState(state)
	if !ok {
		t.Fatal("pair point input")
	}
	return point
}

func contributionPoints(t testing.TB, work *Work, states ...State) []PointState {
	t.Helper()
	result := make([]PointState, len(states))
	for index, state := range states {
		result[index] = contributionPoint(t, work, state)
	}
	return result
}

// contributionPointOf republishes one closed contribution in the nominal point
// role so a law can state a demand-facing point projection over it.
func contributionPointOf(t testing.TB, work *Work, value Contribution) PointState {
	t.Helper()
	rule, ok := work.AsRuleContribution(value)
	if !ok {
		t.Fatal("rule role")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("point role")
	}
	return point
}

// contributionSlotWrite names one declared factor write of a published input.
type contributionSlotWrite struct {
	operation *carryOnlyOperation
	slot      shape.Slot
	root      uint64
}

// contributionWrittenPoint publishes the named factor writes through one
// zero-input rule contribution and returns the result as the PointState a
// later input port reads. A Rule input is reachable only from a closed rule
// publication, so the fixture builds one instead of relabelling a raw
// committed State as an empty authored surface.
func contributionWrittenPoint(t testing.TB, work *Work, plan ContributionPlan, target Scope, premise support.Mask, writes ...contributionSlotWrite) PointState {
	t.Helper()
	base, ok := work.BeginRuleContribution(plan, target, nil, premise)
	if !ok {
		t.Fatal("begin input publication")
	}
	patches := make([]Patch, 0, len(writes))
	for _, write := range writes {
		patches = append(patches, contributionPatch(t, work, write.operation, base.State(), write.slot, write.root))
	}
	rule, ok := work.FinishRuleContribution(base, patches)
	if !ok {
		t.Fatal("finish input publication")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("publish input point")
	}
	return point
}

func contributionSevered(t testing.TB, base RuleContributionBase) {
	t.Helper()
	owner := base.value
	if owner == nil || owner.live || owner.work != nil || owner.plan != nil || owner.inputs != nil || owner.rootsOwned || owner.state.authority != nil || owner.state.roots != nil || owner.state.support.Valid() || base.State().authority != nil {
		t.Fatal("contribution base retained execution graph after consumption")
	}
}

func TestFinishRuleContributionBorrowsClosedCarryAndEnvironmentRoots(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("support regions")
	}
	operations := []*sparseContributionTripwireOperation{
		{carryOnlyOperation: &carryOnlyOperation{guards: manager}},
		{carryOnlyOperation: &carryOnlyOperation{guards: manager}},
	}
	composition, ok := attachTestComposition(t, []FactorOperation{operations[0], operations[1]})
	if !ok {
		t.Fatal("composition")
	}
	sourcePlan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("source plan")
	}
	plan, ok := composition.SealContribution(1, nil, []ContributionSource{{Slot: 0, Input: 0}}, false, true)
	if !ok {
		t.Fatal("carry/environment plan")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity reindex")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	source := contributionWrittenPoint(t, work, sourcePlan, composition.Scope(), whole,
		contributionSlotWrite{operation: operations[0].carryOnlyOperation, slot: 0, root: 2},
		contributionSlotWrite{operation: operations[1].carryOnlyOperation, slot: 1, root: 2})
	if !source.closed {
		t.Fatal("closed source proof was not retained")
	}
	sourceZero, ok := source.HandleAt(0)
	if !ok {
		t.Fatal("source slot 0")
	}
	sourceOne, ok := source.HandleAt(1)
	if !ok {
		t.Fatal("source slot 1")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{source}, whole, source)
	if !ok {
		t.Fatal("begin")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() || !result.Support().Equal(whole) {
		t.Fatal("finish")
	}
	if operations[0].closeCalls != 0 || operations[1].closeCalls != 0 {
		t.Fatalf("closed carry/environment roots were reclosed: (%d,%d)", operations[0].closeCalls, operations[1].closeCalls)
	}
	gotZero, ok := result.HandleAt(0)
	if !ok || gotZero != sourceZero {
		t.Fatal("carry root changed")
	}
	gotOne, ok := result.HandleAt(1)
	if !ok || gotOne != sourceOne {
		t.Fatal("environment root changed")
	}
	lifted, ok := work.LiftRuleContribution(source)
	if !ok || !work.EqualRuleContribution(result, lifted) {
		t.Fatal("borrowed finish changed the closed semantic surface")
	}

	latent, ok := work.TransportPointState(source, whole, identity, on)
	if !ok || latent.closed {
		t.Fatal("hostile source did not clear the closed proof")
	}
	base, ok = work.BeginRuleContribution(plan, composition.Scope(), []PointState{latent}, on, latent)
	if !ok {
		t.Fatal("latent begin")
	}
	if _, ok = work.FinishRuleContribution(base, nil); !ok {
		t.Fatal("latent finish")
	}
	if operations[0].closeCalls != 1 || operations[1].closeCalls != 1 {
		t.Fatalf("unclosed carry/environment roots skipped close: (%d,%d)", operations[0].closeCalls, operations[1].closeCalls)
	}
}

type contributionPublisher struct {
	reserve   bool
	reserved  int
	published int
	dropped   int
}

func (*contributionPublisher) Ready() bool { return true }

func (publisher *contributionPublisher) Reserve() bool {
	publisher.reserved++
	return publisher.reserve
}

func (publisher *contributionPublisher) Publish() RootHandle {
	publisher.published++
	return RootHandle{}
}

func (publisher *contributionPublisher) Drop() { publisher.dropped++ }

func contributionPendingPatch(t testing.TB, work *Work, operation *carryOnlyOperation, predecessor State, slot shape.Slot, whole support.Mask, publisher RootPublisher) Patch {
	t.Helper()
	before, ok := predecessor.HandleAt(slot)
	if !ok {
		t.Fatal("pending patch before root")
	}
	change, ok := operation.issuer.IssueChange(before, RootHandle{}, publisher, whole, nil, nil, nil)
	if !ok {
		t.Fatal("pending change")
	}
	patch, ok := work.Accept(predecessor, change)
	if !ok {
		t.Fatal("accepted pending patch")
	}
	return patch
}

func TestContributionZeroInputFactorIngress(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal zero-input factor ingress")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), nil, whole)
	if !ok || !work.OwnsRuleContributionBase(base, nil) || !base.State().Scope().Same(composition.Scope()) {
		t.Fatal("begin zero-input ingress")
	}
	before, _ := initial.HandleAt(0)
	baseRoot, _ := base.State().HandleAt(0)
	if baseRoot != before {
		t.Fatal("zero-input ingress did not start from initial root")
	}
	patch := contributionPatch(t, work, operations[0], base.State(), 0, 2)
	result, ok := work.FinishRuleContribution(base, []Patch{patch})
	if !ok || !result.Valid() || !result.Support().Equal(whole) {
		t.Fatal("finish zero-input ingress")
	}
	after, _ := result.HandleAt(0)
	if after == before {
		t.Fatal("zero-input ingress did not publish its declared factor patch")
	}
	contributionSevered(t, base)
}

func TestContributionZeroInputIngressUsesExplicitStrictTargetScope(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	target, ok := composition.SealScope([]guard.Atom{1})
	if !ok {
		t.Fatal("strict target scope")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	within, ok := build.Literal(1, true)
	if !ok || !build.Seal() {
		t.Fatal("strict target support")
	}
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("zero-input ingress plan")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	base, ok := work.BeginRuleContribution(plan, target, nil, within)
	if !ok || !base.State().Scope().Same(target) || base.State().Scope().Same(composition.Scope()) {
		t.Fatal("zero-input ingress did not retain its explicit strict target scope")
	}
	patch := contributionPatch(t, work, operation, base.State(), 0, 2)
	result, ok := work.FinishRuleContribution(base, []Patch{patch})
	if !ok || !result.Valid() || !result.Scope().Same(target) || !result.Support().Equal(within) {
		t.Fatal("strict-scope zero-input ingress did not publish exactly in its target scope")
	}
	contributionSevered(t, base)
}

func TestContributionZeroInputStructuralIngressUsesInitialRoots(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(0, nil, nil, true)
	if !ok {
		t.Fatal("seal zero-input structural ingress")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	changed := contributionWrite(t, work, operations[0], initial, 0, 2)
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin zero-input structural ingress")
	}
	baseRoot, _ := base.State().HandleAt(0)
	initialRoot, _ := initial.HandleAt(0)
	changedRoot, _ := changed.HandleAt(0)
	if baseRoot != initialRoot || baseRoot == changedRoot {
		t.Fatal("zero-input structural ingress inherited a predecessor root")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() || !result.Support().Equal(whole) {
		t.Fatal("finish zero-input structural ingress")
	}
	contributionSevered(t, base)
}

func TestContributionMultiInputStructuralPrune(t *testing.T) {
	manager, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(3, nil, nil, true)
	if !ok {
		t.Fatal("seal multi-input structural prune")
	}
	inputPlan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	first := contributionWrittenPoint(t, work, inputPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[0], slot: 0, root: 2})
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), append([]PointState{first}, contributionPoints(t, work, initial, initial)...), empty)
	if !ok {
		t.Fatal("begin multi-input structural prune")
	}
	baseRoot, _ := base.State().HandleAt(0)
	firstRoot, _ := first.HandleAt(0)
	initialRoot, _ := initial.HandleAt(0)
	if baseRoot != initialRoot || baseRoot == firstRoot {
		t.Fatal("structural prune acquired an undeclared implicit carry")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() || !support.Empty(result.Support()) {
		t.Fatal("finish multi-input structural prune")
	}
	contributionSevered(t, base)
}

func TestContributionAtomicallyCommitsFactorPatchAndStructuralPrune(t *testing.T) {
	manager, whole, composition, operations, _ := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, nil, true)
	if !ok {
		t.Fatal("seal mixed factor/support contribution")
	}
	inputPlan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	input := contributionWrittenPoint(t, work, inputPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[0], slot: 0, root: 2})
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{input}, whole)
	if !ok {
		t.Fatal("begin mixed contribution")
	}
	patch := contributionPatch(t, work, operations[0], base.State(), 0, 1)
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	result, ok := work.FinishRuleContributionWithSupport(base, []Patch{patch}, empty)
	if !ok || !result.Valid() || !support.Empty(result.Support()) {
		t.Fatal("mixed contribution did not publish the atomic retained support")
	}
	inputRoot, _ := input.HandleAt(0)
	resultRoot, _ := result.HandleAt(0)
	if resultRoot == inputRoot {
		t.Fatal("mixed contribution lost its Factor patch while pruning support")
	}
	contributionSevered(t, base)
}

func TestContributionCarriesFromInputBeyondSecondPort(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(3, []shape.Slot{0}, []ContributionSource{{Slot: 0, Input: 2}}, false)
	if !ok {
		t.Fatal("seal third-input carry")
	}
	inputPlan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	third := contributionWrittenPoint(t, work, inputPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[0], slot: 0, root: 2})
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), append(contributionPoints(t, work, initial, initial), third), whole)
	if !ok {
		t.Fatal("begin third-input carry")
	}
	got, _ := base.State().HandleAt(0)
	want, _ := third.HandleAt(0)
	if got != want {
		t.Fatal("carry did not use input port two")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() {
		t.Fatal("finish third-input carry")
	}
	got, _ = result.HandleAt(0)
	if got != want {
		t.Fatal("finished carry lost input port two root")
	}
	contributionSevered(t, base)
}

func TestContributionFailedPublicationIsAtomicAndConsumesBase(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 2)
	plan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("seal zero-input two-factor ingress")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("begin")
	}
	first := &contributionPublisher{reserve: true}
	second := &contributionPublisher{reserve: false}
	firstPatch := contributionPendingPatch(t, work, operations[0], base.State(), 0, whole, first)
	secondPatch := contributionPendingPatch(t, work, operations[1], base.State(), 1, whole, second)
	if _, ok := work.FinishRuleContribution(base, []Patch{firstPatch, secondPatch}); ok {
		t.Fatal("finish accepted a failed pending publication")
	}
	if first.reserved != 1 || second.reserved != 1 || first.published != 0 || second.published != 0 || first.dropped != 1 || second.dropped != 1 {
		t.Fatal("failed publication reserved, published, or dropped an incomplete batch incorrectly")
	}
	initialFirst, _ := initial.HandleAt(0)
	initialSecond, _ := initial.HandleAt(1)
	if now, _ := initial.HandleAt(0); now != initialFirst {
		t.Fatal("failed publication changed first initial root")
	}
	if now, _ := initial.HandleAt(1); now != initialSecond {
		t.Fatal("failed publication changed second initial root")
	}
	contributionSevered(t, base)
}

func TestContributionProjectsOnlyDeclaredCarries(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 2)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, []ContributionSource{{Slot: 0, Input: 0}}, false)
	if !ok {
		t.Fatal("seal contribution")
	}
	inputPlan, ok := composition.SealContribution(0, []shape.Slot{0, 1}, nil, false)
	if !ok {
		t.Fatal("seal input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	input := contributionWrittenPoint(t, work, inputPlan, composition.Scope(), whole,
		contributionSlotWrite{operation: operations[0], slot: 0, root: 2},
		contributionSlotWrite{operation: operations[1], slot: 1, root: 2})
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{input}, whole)
	if !ok || !work.OwnsRuleContributionStates(base, []State{input.State()}) {
		t.Fatal("begin contribution")
	}
	projected := base.State()
	if projected.Valid() {
		t.Fatal("unpublishable base reported public validity")
	}
	if _, _, committed := work.Commit(projected, nil); committed {
		t.Fatal("ordinary commit accepted contribution base")
	}
	carried, _ := projected.HandleAt(0)
	written, _ := input.HandleAt(0)
	if carried != written {
		t.Fatal("declared carry did not retain its exact source root")
	}
	unrelated, _ := projected.HandleAt(1)
	initialUnrelated, _ := initial.HandleAt(1)
	inputUnrelated, _ := input.HandleAt(1)
	if unrelated != initialUnrelated || unrelated == inputUnrelated {
		t.Fatal("undeclared input root leaked into contribution")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() || !result.Support().Equal(whole) {
		t.Fatal("finish contribution")
	}
	resultCarried, _ := result.HandleAt(0)
	resultUnrelated, _ := result.HandleAt(1)
	if resultCarried != written || resultUnrelated != initialUnrelated || base.State().authority != nil {
		t.Fatal("finished contribution leaked or retained temporary state")
	}
	contributionSevered(t, base)
}

func TestContributionSharesOnePatchPredecessorAndDropsOwnedDisallowedSlot(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 3)
	plan, ok := composition.SealContribution(2, []shape.Slot{0, 1}, []ContributionSource{{Slot: 0, Input: 0}, {Slot: 1, Input: 1}}, false)
	if !ok {
		t.Fatal("seal two-input contribution")
	}
	leftPlan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal left input publication")
	}
	rightPlan, ok := composition.SealContribution(0, []shape.Slot{1}, nil, false)
	if !ok {
		t.Fatal("seal right input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	left := contributionWrittenPoint(t, work, leftPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[0], slot: 0, root: 2})
	right := contributionWrittenPoint(t, work, rightPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[1], slot: 1, root: 2})
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{left, right}, whole)
	if !ok || work.OwnsRuleContributionStates(base, []State{initial, right.State()}) {
		t.Fatal("contribution input fence")
	}
	first := contributionPatch(t, work, operations[0], base.State(), 0, 1)
	second := contributionPatch(t, work, operations[1], base.State(), 1, 1)
	if !sameState(first.state, second.state) || !sameState(first.state, base.State()) {
		t.Fatal("siblings did not share exact projected predecessor")
	}
	result, ok := work.FinishRuleContribution(base, []Patch{first, second})
	if !ok || !result.Valid() {
		t.Fatal("finish shared-base patches")
	}

	base, ok = work.BeginRuleContribution(plan, composition.Scope(), []PointState{left, right}, whole)
	if !ok {
		t.Fatal("foreign-predecessor base")
	}
	foreignPredecessor := contributionPatch(t, work, operations[0], left.State(), 0, 1)
	if _, ok := work.FinishRuleContribution(base, []Patch{foreignPredecessor}); ok {
		t.Fatal("finish accepted a patch from an equal-looking source state")
	}
	if foreignPredecessor.change.record == nil || foreignPredecessor.change.record.consumed || !work.Discard(foreignPredecessor) {
		t.Fatal("foreign predecessor patch was consumed or could not be cleaned")
	}

	base, ok = work.BeginRuleContribution(plan, composition.Scope(), []PointState{left, right}, whole)
	if !ok {
		t.Fatal("second base")
	}
	foreign := contributionPatch(t, work, operations[2], base.State(), 2, 2)
	if _, ok := work.FinishRuleContribution(base, []Patch{foreign}); ok {
		t.Fatal("finish accepted patch outside sealed write membership")
	}
	if foreign.change.record == nil || !foreign.change.record.consumed {
		t.Fatal("owned disallowed-slot patch was not consumed")
	}
	if base.State().authority != nil {
		t.Fatal("failed finish retained contribution base")
	}
}

func TestSupportContributionRestrictsAllRoots(t *testing.T) {
	manager, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, nil, nil, true)
	if !ok {
		t.Fatal("seal support contribution")
	}
	inputPlan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal input publication")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	input := contributionWrittenPoint(t, work, inputPlan, composition.Scope(), whole, contributionSlotWrite{operation: operations[0], slot: 0, root: 2})
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty support")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{input}, empty)
	if !ok {
		t.Fatal("begin support contribution")
	}
	baseRoot, _ := base.State().HandleAt(0)
	inputRoot, _ := input.HandleAt(0)
	initialRoot, _ := initial.HandleAt(0)
	if baseRoot != initialRoot || baseRoot == inputRoot {
		t.Fatal("support contribution acquired an undeclared implicit carry")
	}
	result, ok := work.FinishRuleContribution(base, nil)
	if !ok || !result.Valid() || !support.Empty(result.Support()) {
		t.Fatal("finish support contribution")
	}
	resultRoot, _ := result.HandleAt(0)
	if resultRoot != initialRoot {
		t.Fatal("support contribution did not retain the initial root")
	}
	full, ok := work.BeginRuleContribution(plan, composition.Scope(), []PointState{input}, whole)
	if !ok {
		t.Fatal("support contribution rejected full retained input support")
	}
	if !work.AbortRuleContribution(full, nil) {
		t.Fatal("abort full support contribution")
	}
}

func TestContributionAbortConsumesBaseAndRejectsZeroOrForeign(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("seal contribution")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	if work.AbortRuleContribution(RuleContributionBase{}, nil) {
		t.Fatal("zero contribution base accepted")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, work, initial), whole)
	if !ok {
		t.Fatal("begin")
	}
	patch := contributionPatch(t, work, operations[0], base.State(), 0, 2)
	other, ok := composition.NewWork()
	if !ok || other.AbortRuleContribution(base, nil) {
		t.Fatal("foreign work accepted contribution base")
	}
	if !work.AbortRuleContribution(base, []Patch{patch}) || base.State().authority != nil {
		t.Fatal("abort contribution")
	}
	if patch.change.record == nil || !patch.change.record.consumed {
		t.Fatal("abort did not consume staged patch")
	}
	if work.AbortRuleContribution(base, nil) {
		t.Fatal("aborted base remained reusable")
	}
	contributionSevered(t, base)
}

func TestContributionComputesInputIntersectionAndStructuralPremise(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("on support")
	}
	operation := &carryOnlyOperation{guards: manager}
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	plan, ok := composition.SealContribution(2, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("plan")
	}
	zeroPlan, ok := composition.SealContribution(0, nil, nil, false)
	if !ok {
		t.Fatal("zero-input plan")
	}
	left, ok := NewState(composition, composition.Scope(), on)
	if !ok {
		t.Fatal("left")
	}
	right, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("right")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	// The caller supplies only the declared premise.  With P ∩ true inputs,
	// an ordinary group computes P internally rather than accepting a claimed
	// input intersection from the executor.
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, work, left, right), whole)
	if !ok || !base.State().Support().Equal(on) {
		t.Fatal("ordinary contribution did not compute input intersection")
	}
	published, ok := work.FinishRuleContribution(base, nil)
	if !ok || !published.Support().Equal(on) {
		t.Fatal("ordinary contribution did not publish computed intersection")
	}
	trueInput, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("true input")
	}
	premised, ok := work.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, work, trueInput, trueInput), on)
	if !ok || !premised.State().Support().Equal(on) {
		t.Fatal("ordinary contribution did not apply structural premise")
	}
	published, ok = work.FinishRuleContribution(premised, nil)
	if !ok || !published.Support().Equal(on) {
		t.Fatal("ordinary premise support did not survive publication")
	}
	zero, ok := work.BeginRuleContribution(zeroPlan, composition.Scope(), nil, on)
	if !ok || !zero.State().Support().Equal(on) {
		t.Fatal("zero-input contribution did not retain premise")
	}
	published, ok = work.FinishRuleContribution(zero, nil)
	if !ok || !published.Support().Equal(on) {
		t.Fatal("zero-input premise did not survive publication")
	}
}

func TestContributionForeignWorkCannotConsumeOwnerPatch(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("plan")
	}
	ownerWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("owner work")
	}
	base, ok := ownerWork.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, ownerWork, initial), whole)
	if !ok {
		t.Fatal("base")
	}
	patch := contributionPatch(t, ownerWork, operations[0], base.State(), 0, 2)
	foreignWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("foreign work")
	}
	if _, ok := foreignWork.FinishRuleContribution(base, []Patch{patch}); ok {
		t.Fatal("foreign Work finished owner contribution")
	}
	if patch.change.record == nil || patch.change.record.consumed || base.State().authority == nil {
		t.Fatal("foreign Work consumed owner patch or base")
	}
	if !ownerWork.AbortRuleContribution(base, []Patch{patch}) {
		t.Fatal("owner could not clean its retained patch")
	}
	contributionSevered(t, base)
}

func TestContributionForeignPatchRemainsUntouchedAndSeversOwnedBase(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("plan")
	}
	ownerWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("owner work")
	}
	foreignWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("foreign work")
	}
	for _, abort := range []bool{false, true} {
		base, ok := ownerWork.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, ownerWork, initial), whole)
		if !ok {
			t.Fatal("base")
		}
		foreignPatch := contributionPatch(t, foreignWork, operations[0], base.State(), 0, 2)
		if abort {
			if ownerWork.AbortRuleContribution(base, []Patch{foreignPatch}) {
				t.Fatal("abort consumed foreign patch")
			}
		} else if _, ok := ownerWork.FinishRuleContribution(base, []Patch{foreignPatch}); ok {
			t.Fatal("finish consumed foreign patch")
		}
		if foreignPatch.change.record == nil || foreignPatch.change.record.consumed {
			t.Fatal("foreign patch was consumed")
		}
		contributionSevered(t, base)
		if !foreignWork.Discard(foreignPatch) {
			t.Fatal("foreign patch cleanup")
		}
	}
}

func TestSupportContributionForeignPatchRemainsCallerOwned(t *testing.T) {
	_, whole, composition, operations, initial := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(1, []shape.Slot{0}, nil, true)
	if !ok {
		t.Fatal("support plan")
	}
	ownerWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("owner work")
	}
	foreignWork, ok := composition.NewWork()
	if !ok {
		t.Fatal("foreign work")
	}
	base, ok := ownerWork.BeginRuleContribution(plan, composition.Scope(), contributionPoints(t, ownerWork, initial), whole)
	if !ok {
		t.Fatal("base")
	}
	foreign := contributionPatch(t, foreignWork, operations[0], base.State(), 0, 2)
	if _, finished := ownerWork.FinishRuleContributionWithSupport(base, []Patch{foreign}, whole); finished {
		t.Fatal("support finish consumed foreign patch")
	}
	if foreign.change.record == nil || foreign.change.record.consumed {
		t.Fatal("support finish touched caller-owned foreign patch")
	}
	contributionSevered(t, base)
	if !foreignWork.Discard(foreign) {
		t.Fatal("foreign owner could not discard retained patch")
	}
}

func TestContributionCancellationAtFinishDropsOwnedPatchAndBase(t *testing.T) {
	_, whole, composition, operations, _ := contributionFixture(t, 1)
	plan, ok := composition.SealContribution(0, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("plan")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	live := true
	if !work.SetCheckpoint(func() bool { return live }) {
		t.Fatal("checkpoint")
	}
	base, ok := work.BeginRuleContribution(plan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("base")
	}
	publisher := &contributionPublisher{reserve: true}
	patch := contributionPendingPatch(t, work, operations[0], base.State(), 0, whole, publisher)
	live = false
	if _, finished := work.FinishRuleContribution(base, []Patch{patch}); finished {
		t.Fatal("cancelled finish published")
	}
	if publisher.reserved != 0 || publisher.published != 0 || publisher.dropped != 1 {
		t.Fatalf("cancelled finish cleanup = reserve:%d publish:%d drop:%d", publisher.reserved, publisher.published, publisher.dropped)
	}
	contributionSevered(t, base)
}
