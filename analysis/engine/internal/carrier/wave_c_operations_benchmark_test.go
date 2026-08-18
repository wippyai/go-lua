package carrier

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// BenchmarkWaveCCarrierOperations is the carrier half of the Wave-C covering
// matrix.  Unlike an engine Point alias, its shared/independent rows are real
// Guard partitions: shared reuses one literal mask, independent uses two
// disjoint literals from the same manager.  The direct Commit at the end is
// the observable delta-publication cut; ChangeSet rows/factors are reported
// without introducing a counter API.
func BenchmarkWaveCCarrierOperations(b *testing.B) {
	cases := []struct{ factors, inputs int }{{3, 1}, {9, 2}, {16, 4}, {25, 4}}
	for _, matrix := range cases {
		for _, shared := range []bool{true, false} {
			matrix, shared := matrix, shared
			name := "factors=" + strconv.Itoa(matrix.factors) + "/inputs=" + strconv.Itoa(matrix.inputs) + "/guards=" + waveCGuardName(shared)
			b.Run(name, func(b *testing.B) {
				var deltaRows, deltaFactors int
				b.ReportAllocs()
				for iteration := 0; iteration < b.N; iteration++ {
					b.StopTimer()
					fixture := newWaveCCarrierOperationFixture(b, matrix.factors, matrix.inputs, shared)
					b.StartTimer()
					rows, factors := fixture.run(b)
					deltaRows += rows
					deltaFactors += factors
				}
				b.StopTimer()
				b.ReportMetric(float64(matrix.factors), "factor-width/op")
				b.ReportMetric(float64(matrix.inputs), "product-inputs/op")
				b.ReportMetric(boolMetricCarrier(shared), "shared-guard-regions/op")
				b.ReportMetric(boolMetricCarrier(!shared), "independent-guard-regions/op")
				b.ReportMetric(float64(deltaRows)/float64(b.N), "delta-units/op")
				b.ReportMetric(float64(deltaFactors)/float64(b.N), "delta-factors/op")
			})
		}
	}
}

type waveCCarrierOperationFixture struct {
	work                    *Work
	whole, left, right, all State
	view                    View
	join, widen, narrow     MergeScope
	plan                    ContributionPlan
	inputs                  int
	operations              []*waveCDeltaOperation
}

func newWaveCCarrierOperationFixture(tb testing.TB, factors, inputs int, shared bool) waveCCarrierOperationFixture {
	tb.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		tb.Fatal("Wave-C guard manager")
	}
	operations := make([]FactorOperation, factors)
	owned := make([]*waveCDeltaOperation, factors)
	for index := range owned {
		owned[index] = newWaveCDeltaOperation(tb, manager)
		operations[index] = owned[index]
	}
	composition, attached := attachTestComposition(tb, operations)
	if !attached {
		tb.Fatal("Wave-C composition")
	}
	regions := support.New(manager)
	leftMask, leftOK := regions.Literal(1, true)
	rightMask, rightOK := regions.Literal(2, true)
	wholeMask, wholeOK := support.True(manager)
	if !leftOK || !rightOK || !wholeOK || !regions.Seal() {
		tb.Fatal("Wave-C guard regions")
	}
	if shared {
		rightMask = leftMask
	}
	left, leftState := NewState(composition, composition.Scope(), leftMask)
	right, rightState := NewState(composition, composition.Scope(), rightMask)
	whole, wholeState := NewState(composition, composition.Scope(), wholeMask)
	if !leftState || !rightState || !wholeState {
		tb.Fatal("Wave-C guarded states")
	}
	view, restricted := whole.Restrict(leftMask)
	join := composition.AllMergeScope()
	widen, widenOK := composition.SealWidening(nil)
	narrow, narrowOK := composition.SealNarrowing(nil)
	plan, planOK := composition.SealContribution(inputs, nil, nil, true)
	if !restricted || !widenOK || !narrowOK || !planOK {
		tb.Fatal("Wave-C operation setup")
	}
	work, workOK := composition.NewWork()
	if !workOK {
		tb.Fatal("Wave-C work")
	}
	return waveCCarrierOperationFixture{work: work, whole: whole, left: left, right: right, all: whole, view: view, join: join, widen: widen, narrow: narrow, plan: plan, inputs: inputs, operations: owned}
}

func (fixture waveCCarrierOperationFixture) run(tb testing.TB) (int, int) {
	tb.Helper()
	// Restrict, carrier Product (Begin/FinishRuleContribution), equality and
	// order.
	if !fixture.work.OwnsViewOf(fixture.whole, fixture.view) || !fixture.work.EqualUnder(fixture.whole, fixture.whole) || !fixture.work.LessOrEqUnder(fixture.left, fixture.all) {
		tb.Fatal("Wave-C restrict/equality/order")
	}
	inputs := make([]State, fixture.inputs)
	for index := range inputs {
		inputs[index] = fixture.whole
	}
	base, begun := fixture.work.BeginRuleContribution(fixture.plan, fixture.whole.Scope(), contributionPoints(tb, fixture.work, inputs...), fixture.whole.Support())
	if !begun {
		tb.Fatal("Wave-C product begin")
	}
	if _, finished := fixture.work.FinishRuleContribution(base, nil); !finished {
		tb.Fatal("Wave-C product finish")
	}

	// Join, Widen, Narrow, and Mu (MergeSelectedPointState) are direct carrier
	// calls. Empty authored target scopes are valid exact-reset selections and
	// deliberately avoid inventing a private target capability.
	if _, _, ok := fixture.work.Merge3Under(Join, fixture.left, fixture.right, fixture.join); !ok {
		tb.Fatal("Wave-C join")
	}
	if _, _, ok := fixture.work.Merge3Under(Widen, fixture.left, fixture.all, fixture.widen); !ok {
		tb.Fatal("Wave-C widen")
	}
	if _, _, ok := fixture.work.Merge3Under(Narrow, fixture.all, fixture.left, fixture.narrow); !ok {
		tb.Fatal("Wave-C narrow")
	}
	widenCurrent, widenRight := selectedPointOperands(tb, fixture.work, fixture.left, fixture.all)
	if _, _, ok := fixture.work.MergeSelectedPointState(Widen, widenCurrent, widenRight, widenRight, fixture.widen); !ok {
		tb.Fatal("Wave-C widen mu")
	}
	narrowCurrent, narrowRight := selectedPointOperands(tb, fixture.work, fixture.all, fixture.left)
	if _, _, ok := fixture.work.MergeSelectedPointState(Narrow, narrowCurrent, narrowRight, narrowRight, fixture.narrow); !ok {
		tb.Fatal("Wave-C narrow mu")
	}

	// This final single-Factor patch is the one actual publication in the
	// sample. Its ChangeSet is inspected only through its public carrier API.
	patch := waveCDeltaPatch(tb, fixture.work, fixture.operations[0], fixture.whole, 0, fixture.left.Support())
	_, changed, committed := fixture.work.Commit(fixture.whole, []Patch{patch})
	if !committed || changed.Count() != 1 || changed.FactorCount() != 1 {
		tb.Fatal("Wave-C delta publication")
	}
	return changed.Count(), changed.FactorCount()
}

func waveCGuardName(shared bool) string {
	if shared {
		return "shared"
	}
	return "independent"
}

func boolMetricCarrier(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// waveCDeltaOperation is deliberately small but semantically nonempty: it
// owns one exact Unit and publishes a distinct root. This lets the benchmark
// validate ChangeSet's sparse exact-unit/factor rows without importing the
// factbinding implementation into carrier (which would create a package
// cycle). Its empty target scope is used only for direct merge dispatch.
type waveCDeltaOperation struct {
	guards   *guard.Manager
	issuer   Issuer
	unit     Unit
	prepared bool
}

func newWaveCDeltaOperation(tb testing.TB, guards *guard.Manager) *waveCDeltaOperation {
	tb.Helper()
	issuer, issued := NewIssuer()
	if !issued {
		tb.Fatal("Wave-C delta issuer")
	}
	unit, unitOK := issuer.IssueUnit(ExactUnit, 1, 1)
	if !unitOK {
		tb.Fatal("Wave-C delta unit")
	}
	return &waveCDeltaOperation{guards: guards, issuer: issuer, unit: unit}
}

func (operation *waveCDeltaOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.prepared {
		return nil, false
	}
	operation.prepared = true
	return operation, true
}
func (operation *waveCDeltaOperation) Guards() *guard.Manager { return operation.guards }
func (operation *waveCDeltaOperation) Attach(owner SlotOwner) RootHandle {
	operation.issuer.Attach(owner)
	root, ok := operation.issuer.IssueRoot(1)
	if !ok {
		panic("Wave-C delta root")
	}
	return root
}
func (operation *waveCDeltaOperation) InitialRootReady() bool {
	return operation != nil && operation.prepared
}
func (operation *waveCDeltaOperation) ValidRoot(root RootHandle) bool {
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && (id == 1 || id == 2)
}
func (operation *waveCDeltaOperation) DeclaredUnit(unit Unit) bool {
	return operation != nil && operation.unit.Same(unit)
}
func (*waveCDeltaOperation) DeclaredTarget(Target) bool                { return false }
func (*waveCDeltaOperation) TargetNotifications(Target) ([]Unit, bool) { return nil, false }
func (*waveCDeltaOperation) PrepareWidening([]Target) (uint64, bool)   { return 0, false }
func (*waveCDeltaOperation) PrepareNarrowing([]Target) (uint64, bool)  { return 0, false }
func (operation *waveCDeltaOperation) ValidUnit(unit Unit) bool {
	return operation != nil && operation.unit.Same(unit)
}
func (*waveCDeltaOperation) ValidTarget(Target) bool { return false }
func (*waveCDeltaOperation) Supports(MergeKind) bool { return true }
func (operation *waveCDeltaOperation) NewWork() (SlotWork, bool) {
	return waveCDeltaWork{issuer: operation.issuer}, true
}

type waveCDeltaWork struct{ issuer Issuer }

func (waveCDeltaWork) SetCheckpoint(Checkpoint) bool                          { return true }
func (waveCDeltaWork) EqualUnder(left, right RootHandle, _ support.Mask) bool { return left == right }
func (waveCDeltaWork) LessOrEqUnder(left, right RootHandle, _ support.Mask) bool {
	return left == right
}
func (waveCDeltaWork) BeginObservation() bool { return false }
func (waveCDeltaWork) EndObservation() bool   { return false }
func (waveCDeltaWork) ObserveUnder(RootHandle, Unit, support.Mask, func(ObservationRow) bool) bool {
	return false
}
func (work waveCDeltaWork) Merge3Under(_ MergeKind, _ bool, _ uint64, left, _ RootHandle, _ support.Split, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) MergeContributionUnder(left, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work waveCDeltaWork) OverlayPointRHSUnder(left, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work waveCDeltaWork) LessOrEqContributionUnder(_, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage) (bool, bool) {
	return true, true
}

func (work waveCDeltaWork) AscentOrderedContributionUnder(_, _ RootHandle, _, _ support.Mask, _, _ SlotCoverage) (bool, bool) {
	return true, true
}

func (work waveCDeltaWork) ContributionClosedUnder(RootHandle, support.Mask, SlotCoverage) bool {
	return true
}

func (work waveCDeltaWork) ContributionPresenceIncludedUnder(support.Mask, support.Mask, SlotCoverage, SlotCoverage) bool {
	return true
}

func (work waveCDeltaWork) MergeTransportedPointUnder(left, _ RootHandle, _, _, _, _ support.Mask, _ guard.Reindex, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}

func (work waveCDeltaWork) ReindexContributionUnder(left RootHandle, _, _ support.Mask, _ guard.Reindex, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) ReindexPointContributionUnder(left RootHandle, _, _ support.Mask, _ guard.Reindex, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) CloseContributionUnder(left, _ RootHandle, _ support.Split, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) MergeSelectedContributionUnder(_ MergeKind, _ uint64, left, _, right RootHandle, _, _ support.Split, _, _, _ SlotCoverage, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, right, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) ReindexUnder(left RootHandle, _ support.Mask, _ support.Mask, _ guard.Reindex, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, left, nil, support.Mask{}, nil, nil, delta)
}
func (work waveCDeltaWork) ReplaceUnder(left, right RootHandle, _ support.Split, delta *support.Work) (ChangeHandle, bool) {
	return work.issuer.IssueChange(left, right, nil, support.Mask{}, nil, nil, delta)
}

func waveCDeltaPatch(tb testing.TB, work *Work, operation *waveCDeltaOperation, predecessor State, slot shape.Slot, region support.Mask) Patch {
	tb.Helper()
	before, beforeOK := predecessor.HandleAt(slot)
	after, afterOK := operation.issuer.IssueRoot(2)
	if !beforeOK || !afterOK {
		tb.Fatal("Wave-C delta roots")
	}
	change, changed := operation.issuer.IssueChange(before, after, nil, region, []Unit{operation.unit}, []support.Mask{region}, nil)
	if !changed {
		tb.Fatal("Wave-C delta change")
	}
	patch, accepted := work.Accept(predecessor, change)
	if !accepted {
		tb.Fatal("Wave-C delta accept")
	}
	return patch
}
