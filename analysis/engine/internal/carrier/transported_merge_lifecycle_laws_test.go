package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestMergeTransportedPointContributionDropsPreparedRootOnCancellation places the
// cancellation edge after the fused typed operation has returned its pending
// publisher. The ordinary carrier commit cut must drop that publisher and
// leave the predecessor untouched; a transport-specific publication route is
// not allowed to leak a prepared root.
func TestMergeTransportedPointContributionDropsPreparedRootOnCancellation(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	live := true
	operation := &transportPendingOperation{guards: manager}
	operation.cancel = &live
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	scope := composition.Scope()
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("empty")
	}
	expression, ok := scope.Expr(empty)
	if !ok {
		t.Fatal("expression")
	}
	builder, ok := composition.NewReindex(scope, scope)
	if !ok || !builder.Set(1, expression) {
		t.Fatal("complement relation")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("relation seal")
	}
	state, ok := NewState(composition, scope, whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	left, ok := work.EmptyContribution(state)
	if !ok {
		t.Fatal("left")
	}
	coverage := contributionCoverage{composition: composition, slots: []slotCoverage{{targets: []TargetRegion{{target: operation.target, region: whole}}}}}
	right, ok := work.admitContribution(state, coverage)
	if !ok {
		t.Fatal("right")
	}
	if !work.SetCheckpoint(func() bool { return live }) {
		t.Fatal("checkpoint")
	}
	if _, _, merged := work.MergeTransportedPointContribution(left, right, whole, plan, whole); merged {
		t.Fatal("canceled fused merge committed")
	}
	if operation.publisher == nil || operation.publisher.dropped != 1 || operation.publisher.published != 0 || operation.publisher.reserved != 0 {
		t.Fatalf("pending publisher lifecycle = %+v, want dropped=1 published=0 reserved=0", operation.publisher)
	}
	if !state.Support().Equal(whole) {
		t.Fatal("predecessor support changed")
	}
	root, ok := state.HandleAt(shape.Slot(0))
	if !ok || root != left.state.roots[0] {
		t.Fatal("predecessor root changed")
	}
}

type transportPendingOperation struct {
	*sparseContributionTripwireOperation
	guards    *guard.Manager
	cancel    *bool
	publisher *transportPendingPublisher
}

func (operation *transportPendingOperation) Preflight() (SlotOperation, bool) {
	if operation == nil {
		return nil, false
	}
	if operation.sparseContributionTripwireOperation == nil {
		operation.sparseContributionTripwireOperation = &sparseContributionTripwireOperation{
			carryOnlyOperation: &carryOnlyOperation{guards: operation.guards},
			withTarget:         true,
		}
	}
	if _, ok := operation.sparseContributionTripwireOperation.Preflight(); !ok {
		return nil, false
	}
	return operation, true
}

func (operation *transportPendingOperation) NewWork() (SlotWork, bool) {
	if operation == nil || operation.carryOnlyOperation == nil {
		return nil, false
	}
	return &transportPendingWork{carryOnlyWork: &carryOnlyWork{issuer: operation.issuer}, operation: operation}, true
}

type transportPendingWork struct {
	*carryOnlyWork
	operation *transportPendingOperation
}

func (work *transportPendingWork) MergeTransportedPointUnder(left, _ RootHandle, _, _, _, _ support.Mask, _ guard.Reindex, _, _ SlotCoverage, candidate *support.Work) (ChangeHandle, bool) {
	if work == nil || work.operation == nil || candidate == nil || !candidate.Open() {
		return ChangeHandle{}, false
	}
	publisher := &transportPendingPublisher{}
	change, ok := work.issuer.IssueChange(left, RootHandle{}, publisher, support.Mask{}, nil, nil, candidate)
	if !ok {
		return ChangeHandle{}, false
	}
	work.operation.publisher = publisher
	if work.operation.cancel != nil {
		*work.operation.cancel = false
	}
	// The pending ChangeHandle is deliberately returned first; carrier owns
	// acceptance and must observe cancellation at the subsequent commit cut.
	return change, true
}

type transportPendingPublisher struct {
	reserved  int
	published int
	dropped   int
}

func (*transportPendingPublisher) Ready() bool { return true }

func (publisher *transportPendingPublisher) Reserve() bool {
	publisher.reserved++
	return true
}

func (publisher *transportPendingPublisher) Publish() RootHandle {
	publisher.published++
	return RootHandle{}
}

func (publisher *transportPendingPublisher) Drop() { publisher.dropped++ }
