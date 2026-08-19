package engine

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func TestBorrowedQueryResultIsSharedAndDetachmentIsExplicit(t *testing.T) {
	solver, query, state := newBorrowedQueryFixture(t)
	key, keyed := query.PublicationKey()
	first, firstReadable := testSnapshotQueryValue[uint64](solver, state, key)
	second, secondReadable := testSnapshotQueryValue[uint64](solver, state, key)
	if !keyed || !firstReadable || !secondReadable || first != second {
		t.Fatalf("borrowed query values = %d/%t and %d/%t", first, firstReadable, second, secondReadable)
	}
	answer, readable := testSnapshotAnswer(solver, state, state.solved.QueryFamily(), key)
	detached, detachedReadable := DetachAnswer[uint64](answer)
	if !readable || !detachedReadable || detached != first {
		t.Fatalf("detached query = %d/%t", detached, detachedReadable)
	}
	wrong, wrongReadable := DetachAnswer[string](answer)
	if wrongReadable || wrong != "" {
		t.Fatal("a foreign answer type crossed the sealed value boundary")
	}
}

func TestBorrowedResultAccessFailsClosed(t *testing.T) {
	solver, query, state := newBorrowedQueryFixture(t)
	_, keyed := query.PublicationKey()
	if !keyed {
		t.Fatal("published query has no key")
	}
	locator, resolved := state.resolveResult(resultLaneQuery, 0)
	if !resolved || !solver.validBorrow(state, locator) {
		t.Fatal("publishing solver rejected its own result address")
	}
	foreignSolver, _, foreignState := newBorrowedQueryFixture(t)
	if foreignSolver.store == solver.store || foreignSolver.validBorrow(state, locator) || solver.validBorrow(foreignState, locator) {
		t.Fatal("result address crossed a solver store fence")
	}
	base := solver.relation
	next, published := solver.runtime.topology.Publish(base, base.Rows())
	if !published || !base.Precedes(next) {
		t.Fatal("solver relation did not advance")
	}
	solver.relation = next
	if solver.validBorrow(state, locator) || solver.ownsCompletedState(state) {
		t.Fatal("a superseded relation retained a readable result")
	}
	solver.relation = base
	if !solver.validBorrow(state, locator) {
		t.Fatal("restoring the publishing relation did not restore the address")
	}
	if solver.validBorrow(state, resultLocator{}) {
		t.Fatal("zero result address validated")
	}
	stale := identity.NewLocator(state.completion.store, state.completion.serial.Next(), locator.Slot)
	if solver.validBorrow(state, stale) {
		t.Fatal("future completion address validated")
	}
	publishedSnapshot, ok := solver.PublishedSnapshot(state)
	if !ok {
		t.Fatal("published snapshot unavailable")
	}
	view := publishedSnapshot.Snapshot()
	queryPlan, opened := snapshot.OpenQuery[identity.ContentID, Answer](&view, publishedSnapshot.QueryFamily())
	if !opened {
		t.Fatal("query family did not open")
	}
	if _, status := snapshot.Query(&view, queryPlan, identity.ContentID{0xFF}); status != snapshot.ReadMiss {
		t.Fatalf("unknown query key status=%v", status)
	}
	if _, invalid := state.resolveResult(resultLaneNone, 0); invalid {
		t.Fatal("unnamed lane minted a result address")
	}
}

func TestResultAddressIsNotPersistable(t *testing.T) {
	typeOfSlot := reflect.TypeOf(resultLocator{}.Slot)
	if typeOfSlot.Kind() != reflect.Struct || typeOfSlot.NumMethod() != 0 || reflect.PointerTo(typeOfSlot).NumMethod() != 0 {
		t.Fatalf("result address exposes an encoding method: %v", typeOfSlot)
	}
	for index := 0; index < typeOfSlot.NumField(); index++ {
		if typeOfSlot.Field(index).IsExported() {
			t.Fatalf("result address field %s is exported", typeOfSlot.Field(index).Name)
		}
	}
	_, _, state := newBorrowedQueryFixture(t)
	locator, ok := state.resolveResult(resultLaneQuery, 0)
	if !ok {
		t.Fatal("published result did not receive an address")
	}
	encoded, err := json.Marshal(locator)
	if err != nil {
		t.Fatalf("marshal result address: %v", err)
	}
	var restored resultLocator
	if err := json.Unmarshal(encoded, &restored); err != nil || restored == locator {
		t.Fatalf("result address round-tripped: %s", encoded)
	}
}

func TestBorrowedResultReadAllocatesNothing(t *testing.T) {
	querySolver, query, queryState := newBorrowedQueryFixture(t)
	observationSolver, observation, observationState := newBorrowedObservationFixture(t)
	queryKey, queryOK := query.PublicationKey()
	if !queryOK {
		t.Fatal("query key unavailable")
	}
	readQuery := func() {
		_, _ = testSnapshotQueryValue[uint64](querySolver, queryState, queryKey)
	}
	readObservation := func() {
		_, _ = testSnapshotObservationValue[uint64](observationSolver, observationState, observation.ID)
	}
	readQuery()
	readObservation()
	if allocations := testing.AllocsPerRun(100, readQuery); allocations != 0 {
		t.Fatalf("borrowed query read allocated %v times", allocations)
	}
	if allocations := testing.AllocsPerRun(100, readObservation); allocations != 0 {
		t.Fatalf("borrowed observation read allocated %v times", allocations)
	}
}
