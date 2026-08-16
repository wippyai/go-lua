package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

var (
	sinkBorrowedRows []uint64
	sinkBorrowed     bool
)

// newBorrowedQueryFixture publishes one completed State whose query result
// carries a mutable backing store, which is what makes the difference between a
// borrowed read and a detached one observable.
func newBorrowedQueryFixture(t testing.TB) (*Solver, ReceiptQuery, *State) {
	t.Helper()
	scratch := []uint64{7, 11}
	solver, query := newDiagnosticsReceiptSolverOf(t, false, hotMutableExactQuerySpec(func(_ OrderedCells[uint64]) []uint64 { return scratch }))
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("borrowed query fixture solve = status:%v state:%t", status, state != nil)
	}
	return solver, query, state
}

// newBorrowedObservationFixture is the observation-column counterpart of
// newBorrowedQueryFixture.
func newBorrowedObservationFixture(t testing.TB) (*Solver, ReceiptObservation[[]uint64], *State) {
	t.Helper()
	scratch := []uint64{7, 11}
	fixture := newExactRuleObservationFixture(t, hotMutableExactQuerySpec(func(_ OrderedCells[uint64]) []uint64 { return scratch }))
	observation, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(94), fixture.member)
	solver, solverOK := fixture.compilation.Solver()
	if !attached || !observation.Available() || !solverOK || solver == nil {
		t.Fatal("borrowed observation fixture")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("borrowed observation fixture solve = status:%v state:%t", status, state != nil)
	}
	return solver, observation, state
}

// TestBorrowedQueryResultIsSharedAndDetachmentIsExplicit fixes the borrow
// contract on the query column: repeated reads borrow the one published value
// rather than copying it, and only an explicit detachment produces an owned
// copy the caller may mutate.
func TestBorrowedQueryResultIsSharedAndDetachmentIsExplicit(t *testing.T) {
	solver, query, state := newBorrowedQueryFixture(t)
	first, firstReadable := ReceiptQueryResult[[]uint64](query, solver, state)
	if !firstReadable || len(first) != 2 || first[0] != 7 || first[1] != 11 {
		t.Fatalf("borrowed query read = %#v/%t", first, firstReadable)
	}
	second, secondReadable := ReceiptQueryResult[[]uint64](query, solver, state)
	if !secondReadable || len(second) != 2 || &second[0] != &first[0] {
		t.Fatalf("a borrowed read copied the published result = %#v/%t", second, secondReadable)
	}
	detached, detachedReadable := DetachReceiptQueryResult[[]uint64](query, solver, state)
	if !detachedReadable || len(detached) != 2 || detached[0] != 7 || detached[1] != 11 || &detached[0] == &first[0] {
		t.Fatalf("detached query read = %#v/%t", detached, detachedReadable)
	}
	detached[0], detached[1] = 101, 103
	third, thirdReadable := ReceiptQueryResult[[]uint64](query, solver, state)
	if !thirdReadable || len(third) != 2 || third[0] != 7 || third[1] != 11 {
		t.Fatalf("a detached copy reached the published result = %#v/%t", third, thirdReadable)
	}
	if _, readable := DetachReceiptQueryResult[uint64](query, solver, state); readable {
		t.Fatal("a detachment recovered the published result at a foreign type")
	}
}

// TestBorrowedResultAccessFailsClosed fixes the one validation a borrowed read
// pays. The address must name the store and completion revision that published
// it, the publishing activation relation must still be live, and the lane kind
// and slot bound must hold. Every one of those fails closed, so a retained
// borrow can never read whatever now occupies a slot.
func TestBorrowedResultAccessFailsClosed(t *testing.T) {
	solver, query, state := newBorrowedQueryFixture(t)
	locator, owner, resolved := resolveQueryResult(query, solver, state)
	if !resolved || owner == nil || !locator.Available() {
		t.Fatal("published query does not resolve to an address")
	}
	again, _, _ := resolveQueryResult(query, solver, state)
	if again != locator {
		t.Fatal("one published query resolves to two addresses")
	}
	if !solver.validBorrow(state, locator) {
		t.Fatal("the publishing solver rejected its own address")
	}
	if _, borrowed := state.queryAt(locator, owner, query.identity.Key()); !borrowed {
		t.Fatal("the publishing state rejected its own address")
	}

	foreignSolver, _, foreignState := newBorrowedQueryFixture(t)
	if foreignSolver.store == solver.store || !foreignSolver.store.Available() {
		t.Fatal("two live solvers share one store identity")
	}
	if foreignSolver.validBorrow(state, locator) || solver.validBorrow(foreignState, locator) {
		t.Fatal("an address issued by one store validated against another")
	}
	if _, readable := ReceiptQueryResult[[]uint64](query, foreignSolver, state); readable {
		t.Fatal("a borrowed read crossed into a foreign solver")
	}
	if _, readable := ReceiptQueryResult[[]uint64](query, solver, foreignState); readable {
		t.Fatal("a borrowed read crossed into a foreign completed state")
	}

	// Publishing the next activation relation invalidates every borrow retained
	// against the previous one. The State is untouched; only the address check
	// changes its answer.
	sealed := solver.relation
	published, publishedOK := solver.runtime.topology.Publish(sealed, sealed.Rows())
	if !publishedOK || !sealed.Precedes(published) {
		t.Fatal("solver topology did not advance its publication")
	}
	solver.relation = published
	if solver.validBorrow(state, locator) {
		t.Fatal("an address from a superseded activation relation stayed valid")
	}
	if _, readable := ReceiptQueryResult[[]uint64](query, solver, state); readable {
		t.Fatal("a retained borrow survived a later activation relation")
	}
	solver.relation = sealed
	if _, readable := ReceiptQueryResult[[]uint64](query, solver, state); !readable {
		t.Fatal("restoring the publishing relation did not restore the borrow")
	}

	if solver.validBorrow(state, resultLocator{}) {
		t.Fatal("the zero address validated")
	}
	stale := identity.NewLocator(state.completion.store, state.completion.serial.Next(), locator.Slot)
	if solver.validBorrow(state, stale) {
		t.Fatal("an address naming a later completion validated")
	}

	// Kind and bounds are the address's own checks and are answered by the
	// column, not by the store fence.
	observationAddress, observationResolved := state.resolveResult(resultLaneObservation, 0)
	if !observationResolved {
		t.Fatal("the observation column refused an address")
	}
	if _, borrowed := state.queryAt(observationAddress, owner, query.identity.Key()); borrowed {
		t.Fatal("a query read accepted an observation address")
	}
	if _, borrowed := state.observationAt(locator, nil, identity.ContentID{}); borrowed {
		t.Fatal("an observation read accepted a query address")
	}
	outOfBounds, outOfBoundsResolved := state.resolveResult(resultLaneQuery, len(state.results))
	if !outOfBoundsResolved {
		t.Fatal("the query column refused an out-of-bounds address")
	}
	if _, borrowed := state.queryAt(outOfBounds, owner, query.identity.Key()); borrowed {
		t.Fatal("a query read accepted an out-of-bounds slot")
	}
	if _, unnamed := state.resolveResult(resultLaneNone, 0); unnamed {
		t.Fatal("an unnamed lane minted an address")
	}
	if _, negative := state.resolveResult(resultLaneQuery, -1); negative {
		t.Fatal("a negative coordinate minted an address")
	}
	var unpublished *State
	if _, minted := unpublished.resolveResult(resultLaneQuery, 0); minted {
		t.Fatal("an unpublished state minted an address")
	}
}

// TestResultAddressIsNotPersistable keeps a result locator an address rather
// than an identity. Its coordinate type has no exported field and no method, so
// it cannot be minted, written down as a durable key, or serialized and
// replayed against another store.
func TestResultAddressIsNotPersistable(t *testing.T) {
	slotType := reflect.TypeOf(resultLocator{}.Slot)
	if slotType.Kind() != reflect.Struct {
		t.Fatalf("result address kind = %v, want struct", slotType.Kind())
	}
	if slotType.PkgPath() == "" || slotType.Name() != "resultAddress" {
		t.Fatalf("result address type %s is not the package-private coordinate", slotType)
	}
	for index := 0; index < slotType.NumField(); index++ {
		if slotType.Field(index).IsExported() {
			t.Errorf("result address exposes field %s", slotType.Field(index).Name)
		}
	}
	if slotType.NumMethod() != 0 || reflect.PointerTo(slotType).NumMethod() != 0 {
		t.Fatal("result address carries methods, which is an encoding surface")
	}

	solver, query, state := newBorrowedQueryFixture(t)
	locator, owner, resolved := resolveQueryResult(query, solver, state)
	if !resolved || owner == nil {
		t.Fatal("published query does not resolve to an address")
	}
	encoded, err := json.Marshal(locator)
	if err != nil {
		t.Fatalf("marshal result locator: %v", err)
	}
	var restored resultLocator
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("unmarshal result locator: %v", err)
	}
	if restored == locator {
		t.Fatalf("a result locator round-tripped through serialization: %s", encoded)
	}
	if _, borrowed := state.queryAt(restored, owner, query.identity.Key()); borrowed {
		t.Fatal("a serialized address borrowed a published result")
	}
}

// TestBorrowedResultReadAllocatesNothing is the cost law of the borrow: a
// validated read of a published result allocates nothing, whatever the result
// type carries. Detachment allocates, which is exactly why it is the caller's
// explicit request rather than the price of every read.
func TestBorrowedResultReadAllocatesNothing(t *testing.T) {
	querySolver, query, queryState := newBorrowedQueryFixture(t)
	observationSolver, observation, observationState := newBorrowedObservationFixture(t)
	for name, read := range map[string]func(){
		"query": func() {
			sinkBorrowedRows, sinkBorrowed = ReceiptQueryResult[[]uint64](query, querySolver, queryState)
		},
		"observation": func() {
			sinkBorrowedRows, sinkBorrowed = ReceiptObservationResult[[]uint64](observation, observationSolver, observationState)
		},
	} {
		read()
		if !sinkBorrowed || len(sinkBorrowedRows) != 2 {
			t.Fatalf("%s borrow = %#v/%t", name, sinkBorrowedRows, sinkBorrowed)
		}
		if allocations := testing.AllocsPerRun(200, read); allocations != 0 {
			t.Fatalf("%s borrowed read allocated %v times per read", name, allocations)
		}
	}
	detach := func() {
		sinkBorrowedRows, sinkBorrowed = DetachReceiptQueryResult[[]uint64](query, querySolver, queryState)
	}
	detach()
	if !sinkBorrowed || len(sinkBorrowedRows) != 2 {
		t.Fatalf("detached read = %#v/%t", sinkBorrowedRows, sinkBorrowed)
	}
	if allocations := testing.AllocsPerRun(200, detach); allocations == 0 {
		t.Fatal("a detached read charged nothing, so the borrow law proves nothing")
	}
}
