package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func transportCoverageBatchFixture(t testing.TB) (*Composition, *Work, contributionCoverage, support.Mask, support.Mask, ReindexPlan, *neutralCoverageOperation, *neutralCoverageOperation) {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	first := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	second := &neutralCoverageOperation{carryOnlyOperation: &carryOnlyOperation{guards: manager}}
	composition, ok := attachTestComposition(t, []FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	secondCoordinate, ok := build.Literal(2, true)
	if !ok || !build.Seal() {
		t.Fatal("second coordinate")
	}
	build = support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	notSecondCoordinate, ok := build.Literal(2, false)
	if !ok || !build.Seal() {
		t.Fatal("negated second coordinate")
	}
	targetScope, ok := composition.SealScope([]guard.Atom{2})
	if !ok {
		t.Fatal("target scope")
	}
	reindexBuilder, ok := composition.NewReindex(composition.Scope(), targetScope)
	if !ok || !reindexBuilder.Forget(1) || !reindexBuilder.Identity(2) {
		t.Fatal("projection relation")
	}
	plan, ok := reindexBuilder.Seal()
	if !ok {
		t.Fatal("projection plan")
	}
	input := contributionCoverage{
		composition: composition,
		slots: []slotCoverage{
			{targets: []TargetRegion{{target: first.target, region: secondCoordinate}}},
			{targets: []TargetRegion{{target: second.target, region: notSecondCoordinate}}},
		},
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	return composition, work, input, whole, secondCoordinate, plan, first, second
}

func TestTransportCoverageBatchPreservesNonIdentityRowsAndDropsEmptyRows(t *testing.T) {
	composition, work, input, whole, pre, plan, first, second := transportCoverageBatchFixture(t)
	defer work.Close()
	result, ok := work.transportContributionCoverage(input, pre, plan, whole, whole)
	if !ok {
		t.Fatal("transport coverage")
	}
	if len(result.slots) != composition.Count() || len(result.slots[0].targets) != 1 || len(result.slots[1].targets) != 0 {
		t.Fatalf("transported rows = %#v", result.slots)
	}
	row := result.slots[0].targets[0]
	if !row.target.Same(first.target) || !row.region.Equal(pre) || !row.region.Valid() {
		t.Fatal("nonidentity transported row changed target or exact support")
	}
	if second.target.Same(row.target) {
		t.Fatal("empty transformed row escaped its slot")
	}
}

func TestTransportCoverageBatchCancellationDiscardsCandidate(t *testing.T) {
	_, work, input, whole, pre, plan, _, _ := transportCoverageBatchFixture(t)
	defer work.Close()
	polls := 0
	if !work.SetCheckpoint(func() bool {
		polls++
		return polls < 3
	}) {
		t.Fatal("checkpoint")
	}
	if _, ok := work.transportContributionCoverage(input, pre, plan, whole, whole); ok {
		t.Fatal("cancelled transport coverage succeeded")
	}
	if polls < 3 || work.supportWork == nil || work.supportWork.Open() {
		t.Fatalf("cancellation polls=%d support-open=%v", polls, work.supportWork != nil && work.supportWork.Open())
	}
}
