package fixpoint

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	relationfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestQueueRetargetsPendingInitialWorkWhenPublicationAdvancesRoot(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	base := mustFullRoot(t, fixture.Base())
	want := sealedDependencies(t, fixture)
	if !queue.SeedFull(base) || queue.Len() != len(want) {
		t.Fatalf("initial queue = %d, want every sealed dependency (%d)", queue.Len(), len(want))
	}
	first := mustLaterRoot(t, fixture.BaseToLeftDelta)
	if !queue.SeedLater(first) {
		t.Fatal("base-to-left successor refused")
	}
	if queue.Len() != len(want) {
		t.Fatalf("successor queue = %d, want every unrun dependency retargeted", queue.Len())
	}
	drainLaterDependencies(t, queue, fixture.LeftRoot(), len(want), want...)
}

func TestQueueRetargetsIndependentPendingWorkAfterAnotherDependencyPublishes(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("initial root")
	}
	// This is the state immediately after the left work has been handed to the
	// evaluator by Next but before its publication is fed back to Queue. Keep
	// the right dependency only so the law does not depend on nominal schedule
	// ordering in the fixture.
	retainOnlyPending(t, queue, fixture.DependencyRight())
	delta := mustDelta(t, fixture.BaseToLeftDelta)
	want := expectedSuccessorDependencies(t, fixture, delta, fixture.DependencyRight())
	if !queue.SeedLater(mustLaterRoot(t, fixture.BaseToLeftDelta)) {
		t.Fatal("left successor")
	}
	// Left is newly woken by its changed relation; right is not woken but must
	// still execute against the successor because it had not run at Base.
	drainLaterDependencies(t, queue, fixture.LeftRoot(), len(want), want...)
}

func TestQueueRejectsSameSuccessorTwiceWithoutLosingRetargetedWork(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("initial root")
	}
	later := mustLaterRoot(t, fixture.BaseToLeftDelta)
	if !queue.SeedLater(later) {
		t.Fatal("first successor")
	}
	before := queue.Len()
	if queue.SeedLater(later) {
		t.Fatal("same authenticated successor was admitted twice")
	}
	if queue.Len() != before {
		t.Fatal("duplicate successor mutated the current wake set")
	}
	want := sealedDependencies(t, fixture)
	drainLaterDependencies(t, queue, fixture.LeftRoot(), len(want), want...)
}

func TestQueueChainsDeltasAndNeverReturnsTheIntermediateRoot(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("initial root")
	}
	if !queue.SeedLater(mustLaterRoot(t, fixture.BaseToLeftDelta)) {
		t.Fatal("left successor")
	}
	if !queue.SeedLater(mustLaterRoot(t, fixture.LeftToBothDelta)) {
		t.Fatal("right successor")
	}
	want := sealedDependencies(t, fixture)
	if queue.Len() != len(want) {
		t.Fatalf("chained queue = %d, want every pending dependency on final root (%d)", queue.Len(), len(want))
	}
	drainLaterDependencies(t, queue, fixture.BothRoot(), len(want), want...)
}

func TestQueueAcceptsOnlyTheExactMountedExecutionAndRoots(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	foreign := relationfixture.New(t, 0x72)
	foreignExecution := foreign.Mounted().Arrangement().Execution()
	if other, ok := New(foreignExecution, fixture.Mounted()); ok || other.Execution().Available() {
		t.Fatal("queue accepted a foreign execution under this mount")
	}
	foreignRoot := mustFullRoot(t, foreign.Base())
	if queue.SeedFull(foreignRoot) {
		t.Fatal("queue accepted a foreign mounted root")
	}
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("foreign-root refusal damaged the valid queue")
	}
}

func TestQueueTerminatesWithoutLaterFallbackRescan(t *testing.T) {
	queue, fixture := newQueueFixture(t)
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("initial root")
	}
	initial := sealedDependencies(t, fixture)
	drainQueue(t, queue, len(initial))
	firstDelta := mustDelta(t, fixture.BaseToLeftDelta)
	firstWant := expectedSuccessorDependencies(t, fixture, firstDelta)
	if !queue.SeedLater(mustLaterRoot(t, fixture.BaseToLeftDelta)) {
		t.Fatal("left successor")
	}
	drainQueue(t, queue, len(firstWant))
	secondDelta := mustDelta(t, fixture.LeftToBothDelta)
	secondWant := expectedSuccessorDependencies(t, fixture, secondDelta)
	if !queue.SeedLater(mustLaterRoot(t, fixture.LeftToBothDelta)) {
		t.Fatal("right successor")
	}
	drainQueue(t, queue, len(secondWant))
	if !queue.Empty() || queue.Len() != 0 {
		t.Fatal("queue scheduled hidden work after all exact deltas were consumed")
	}
}

func TestQueueCannotInjectOrInferARecurrencePermit(t *testing.T) {
	typeOfQueue := reflect.TypeOf(Queue{})
	for index := 0; index < typeOfQueue.NumField(); index++ {
		field := typeOfQueue.Field(index)
		if field.Name == "permit" || field.Name == "widening" || field.Name == "heads" {
			t.Fatalf("queue owns recurrence authority field %q", field.Name)
		}
	}
	pointer := reflect.TypeOf((*Queue)(nil))
	for index := 0; index < pointer.NumMethod(); index++ {
		name := pointer.Method(index).Name
		if name == "AttachPermit" || name == "AdmitPermit" {
			t.Fatalf("queue exposes legacy recurrence injection %s", name)
		}
	}
	queue, fixture := newQueueFixture(t)
	if !queue.SeedFull(mustFullRoot(t, fixture.Base())) {
		t.Fatal("initial root")
	}
	work := mustNext(t, queue)
	entry, ok := queue.Entry(work)
	if !ok || entry.WideningFor(fixture.RelationLeft()) {
		t.Fatal("acyclic fixture fabricated a recurrence head")
	}
	if _, ok := fixture.Mounted().Widening(work.Dependency(), fixture.RelationLeft()); ok {
		t.Fatal("mount fabricated an undeclared widening permit")
	}
}

func newQueueFixture(t *testing.T) (*Queue, relationfixture.Fixture) {
	t.Helper()
	fixture := relationfixture.New(t)
	execution := fixture.Mounted().Arrangement().Execution()
	queue, ok := New(execution, fixture.Mounted())
	if !ok {
		t.Fatal("queue fixture")
	}
	return &queue, fixture
}

func mustFullRoot(t *testing.T, version database.Version) Root {
	t.Helper()
	root, ok := Full(version)
	if !ok {
		t.Fatal("full root")
	}
	return root
}

func mustLaterRoot(t *testing.T, source func() (database.Delta, bool)) Root {
	t.Helper()
	delta := mustDelta(t, source)
	root, ok := Later(delta)
	if !ok {
		t.Fatal("later root")
	}
	return root
}

func mustDelta(t *testing.T, source func() (database.Delta, bool)) database.Delta {
	t.Helper()
	delta, ok := source()
	if !ok {
		t.Fatal("fixture delta")
	}
	return delta
}

// sealedDependencies is the law's inventory authority. It redeems the
// mounted Execution schedule rather than repeating a fixture count or
// maintaining a second list when a new dependency family is sealed.
func sealedDependencies(t *testing.T, fixture relationfixture.Fixture) []model.DependencyID {
	t.Helper()
	execution := fixture.Mounted().Arrangement().Execution()
	if !execution.Available() {
		t.Fatal("sealed execution")
	}
	entries := execution.Schedules()
	if len(entries) == 0 {
		t.Fatal("sealed dependency schedule")
	}
	want := make([]model.DependencyID, 0, len(entries))
	for _, entry := range entries {
		if !entry.Available() {
			t.Fatal("unavailable sealed dependency")
		}
		want = append(want, entry.Dependency())
	}
	return want
}

// expectedSuccessorDependencies derives the exact wake set from the sealed
// Execution indexes. Pending work is supplied explicitly to model an
// evaluator item handed out before publication; no queue implementation is
// consulted, so a missing wake remains observable.
func expectedSuccessorDependencies(t *testing.T, fixture relationfixture.Fixture, delta database.Delta, pending ...model.DependencyID) []model.DependencyID {
	t.Helper()
	if !delta.Available() {
		t.Fatal("successor delta")
	}
	execution := fixture.Mounted().Arrangement().Execution()
	if !execution.Available() {
		t.Fatal("sealed execution")
	}
	selected := make(map[model.DependencyID]struct{}, len(pending))
	for _, dependency := range pending {
		entry, ok := execution.Dependency(dependency)
		if !ok || !entry.Available() {
			t.Fatalf("pending dependency is not sealed: %v", dependency)
		}
		selected[dependency] = struct{}{}
	}
	add := func(entry arrangement.ScheduleEntry) {
		if !entry.Available() {
			t.Fatal("unavailable wake entry")
		}
		selected[entry.Dependency()] = struct{}{}
	}
	for _, column := range delta.ChangedColumnIDs() {
		if !column.Available() {
			t.Fatal("unavailable changed column")
		}
		for _, entry := range execution.WakeColumn(column) {
			add(entry)
		}
		for _, entry := range execution.WakeRelation(column.Relation()) {
			add(entry)
		}
	}
	// Preserve the execution's canonical order in the expected vector.
	want := make([]model.DependencyID, 0, len(selected))
	for _, entry := range execution.Schedules() {
		if _, ok := selected[entry.Dependency()]; ok {
			want = append(want, entry.Dependency())
		}
	}
	if len(want) != len(selected) {
		t.Fatal("expected wake set contains a dependency outside the sealed schedule")
	}
	return want
}

func mustNext(t *testing.T, queue *Queue) Work {
	t.Helper()
	work, ok := queue.Next()
	if !ok || !work.Available() {
		t.Fatal("expected queued work")
	}
	return work
}

func assertLaterRoot(t *testing.T, root Root, want database.Version) {
	t.Helper()
	delta, ok := root.Delta()
	if !ok || !delta.Next().Same(want) {
		t.Fatal("work carried the wrong immutable successor root")
	}
}

func drainQueue(t *testing.T, queue *Queue, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		mustNext(t, queue)
	}
	if _, ok := queue.Next(); ok {
		t.Fatal("queue retained unexpected work")
	}
}

func drainLaterDependencies(t *testing.T, queue *Queue, root database.Version, count int, want ...model.DependencyID) {
	t.Helper()
	seen := make(map[model.DependencyID]struct{}, len(want))
	for range count {
		work := mustNext(t, queue)
		assertLaterRoot(t, work.Root(), root)
		if _, duplicate := seen[work.Dependency()]; duplicate {
			t.Fatal("successor queue duplicated a dependency")
		}
		seen[work.Dependency()] = struct{}{}
	}
	for _, dependency := range want {
		if _, found := seen[dependency]; !found {
			t.Fatalf("successor queue omitted dependency %v", dependency)
		}
	}
	if _, ok := queue.Next(); ok {
		t.Fatal("queue retained stale or duplicate successor work")
	}
}

func retainOnlyPending(t *testing.T, queue *Queue, dependency model.DependencyID) {
	t.Helper()
	kept := queue.items[:0]
	for _, work := range queue.items {
		if work.Dependency() == dependency {
			kept = append(kept, work)
		}
	}
	queue.items = kept
	if len(queue.items) != 1 || queue.items[0].Dependency() != dependency {
		t.Fatal("could not construct the one-pending-dependency transition state")
	}
}
