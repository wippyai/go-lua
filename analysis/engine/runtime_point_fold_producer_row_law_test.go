package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
)

// newProducerRowLawEpoch builds one mounted producer plane whose row ordinals
// and Group ordinals deliberately disagree: Group 7 occurs in two states, so
// the head state's row for it is 2 while its Group ordinal is 7. That is the
// exact shape liftStateRegions publishes for a Region head whose Group also
// occurs in a sibling context.
func newProducerRowLawEpoch(t *testing.T, head int) (*executorEpoch, func()) {
	t.Helper()
	fixture := newNewtonLawFixture(t, 1)
	work, workOK := fixture.composition.NewWork()
	if !workOK {
		t.Fatal("producer row law work")
	}
	state, stateOK := carrier.NewState(fixture.composition, fixture.scope, fixture.whole)
	if !stateOK {
		t.Fatal("producer row law state")
	}
	point, pointOK := work.EmptyPointState(state)
	if !pointOK {
		work.Close()
		t.Fatal("producer row law PointState")
	}
	candidate, candidateOK := work.LiftRuleContribution(point)
	if !candidateOK {
		work.Close()
		t.Fatal("producer row law RuleContribution")
	}
	rows := []stateGroupRow{
		{state: 0, group: 7},
		{state: 1, group: 3},
		{state: 1, group: 7},
	}
	byKey := map[stateGroupKey]int{
		{state: 0, group: 7}: 0,
		{state: 1, group: 3}: 1,
		{state: 1, group: 7}: 2,
	}
	index := sealStateGroupIndex(rows, byKey)
	if !index.valid() {
		work.Close()
		t.Fatal("producer row law index seal")
	}
	runtime := &solverRuntime{
		carrier:        fixture.composition,
		artifactBacked: true,
		producerRows:   index,
		producers:      make([]runtimeProducer, 8),
	}
	epoch := &executorEpoch{
		runtime:      runtime,
		work:         work,
		producers:    make([]producerEpoch, len(rows)),
		currentState: head,
	}
	for rowIndex, row := range rows {
		epoch.producers[rowIndex] = producerEpoch{
			state:      row.state,
			group:      row.group,
			generation: 1,
			applied:    1,
			candidate:  candidate,
			hasValue:   true,
		}
	}
	return epoch, func() { work.Close() }
}

// TestPointFoldGroupStateReadsTheProducerRowItsRegionPublished states the one
// index-space law the mounted head fold rests on. A Region's producer operand
// rows are minted as compact producer-cache ordinals by the owner that binds
// them -- liftStateRegions for a mounted frontier -- and buildStateOperandPlane
// transposes the same axis. The fold therefore addresses that slot; resolving
// the ordinal a second time as a graph Group reads a different plane and
// refuses a settled contribution.
func TestPointFoldGroupStateReadsTheProducerRowItsRegionPublished(t *testing.T) {
	epoch, release := newProducerRowLawEpoch(t, 1)
	defer release()
	published := 2
	folded, ok := epoch.pointFoldGroupState(published)
	if !ok || !folded.Valid() {
		t.Fatalf("published producer row %d refused ok=%v valid=%v", published, ok, folded.Valid())
	}
	expected, expectedOK := epoch.work.PointStateFromRuleContribution(epoch.producers[published].candidate)
	if !expectedOK || !epoch.work.EqualPointState(folded, expected) {
		t.Fatalf("published producer row %d folded a foreign contribution", published)
	}
}

// TestPointFoldGroupStateRefusesAProducerRowOfAnotherState is the fail-closed
// half: a Region only folds its own head, so a row owned by a sibling context
// is not a substitute even when its cache holds a settled contribution.
func TestPointFoldGroupStateRefusesAProducerRowOfAnotherState(t *testing.T) {
	epoch, release := newProducerRowLawEpoch(t, 1)
	defer release()
	sibling := 0
	if epoch.runtime.producerRows.rows[sibling].state == contextfiber.StateOrdinal(epoch.currentState) {
		t.Fatalf("row %d is not owned by a sibling state", sibling)
	}
	if _, admitted := epoch.pointFoldGroupState(sibling); admitted {
		t.Fatalf("fold admitted producer row %d owned by state %d while folding state %d", sibling, epoch.runtime.producerRows.rows[sibling].state, epoch.currentState)
	}
}
