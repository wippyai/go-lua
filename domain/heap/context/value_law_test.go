package context_test

import (
	"testing"

	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
	"github.com/wippyai/go-lua/domain/materialization"
)

// TestContextValueKeepsExactReferenceRowsAndTheAllocationKey states the
// factor carrier's central invariant: a cell is a compact immutable set of
// authenticated contextual Reference rows, and every row in one cell names
// the same Heap allocation coordinate.
func TestContextValueKeepsExactReferenceRowsAndTheAllocationKey(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	shared, sharedOK := fixture.schema.Share(local.Result(), fixture.leftRight)
	if !keyOK || !localOK || !sharedOK {
		t.Fatal("context reference fixture")
	}

	left, leftOK := fixture.schema.Exact(local.Result())
	right, rightOK := fixture.schema.Exact(shared.Result())
	joined, joinedOK := fixture.schema.Join(left, right)
	if !leftOK || !rightOK || !joinedOK || joined.IsBottom() || joined.IsTop() {
		t.Fatal("exact context values did not join")
	}
	if joined.ReferenceCount() != 2 {
		t.Fatalf("joined reference count = %d, want 2", joined.ReferenceCount())
	}
	for index := 0; index < joined.ReferenceCount(); index++ {
		row, rowOK := joined.ReferenceAt(index)
		rowKeyID, rowKeyOK := row.Key().ContentID()
		keyID, keyIDOK := key.ContentID()
		if !rowOK || !row.Valid() || !rowKeyOK || !keyIDOK || rowKeyID != keyID {
			t.Fatalf("joined row %d lost its exact allocation key", index)
		}
	}
	if !fixture.schema.Admit(key, joined) {
		t.Fatal("context value was not admitted at its exact allocation key")
	}
	other, otherOK := fixture.schema.FreshAt(0)
	if !otherOK {
		t.Fatal("fresh allocation fixture")
	}
	if fixture.schema.Admit(other, joined) {
		t.Fatal("context value crossed an allocation-key coordinate")
	}
}

// TestContextValueLatticeIsMonotoneAndOwnerFenced states the lattice laws
// needed by the engine Factor boundary. Bottom carries no reference, Join
// preserves exact alternatives, and Top is an explicit owner-issued value;
// foreign issuers cannot enter any operation.
func TestContextValueLatticeIsMonotoneAndOwnerFenced(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	if !keyOK || !localOK {
		t.Fatal("context reference fixture")
	}
	exact, exactOK := fixture.schema.Exact(local.Result())
	bottom := fixture.schema.Bottom()
	top := fixture.schema.Top()
	if !exactOK || !bottom.Valid() || !top.Valid() || !bottom.IsBottom() || !top.IsTop() {
		t.Fatal("context lattice extremes")
	}
	if !fixture.schema.LessOrEq(bottom, exact) || !fixture.schema.LessOrEq(exact, top) || fixture.schema.LessOrEq(top, exact) {
		t.Fatal("context lattice order")
	}
	joined, joinedOK := fixture.schema.Join(exact, exact)
	if !joinedOK || !fixture.schema.Equal(joined, exact) || joined.ReferenceCount() != 1 {
		t.Fatal("context Join was not idempotent")
	}
	widened, widenedOK := fixture.schema.Widen(exact, joined)
	if !widenedOK || !fixture.schema.Equal(widened, exact) {
		t.Fatal("context Widen changed a stationary exact value")
	}
	foreign, foreignOK := contextdomain.Seal(fixture.heap, fixture.schema.Directory())
	if !foreignOK || foreign.OwnsSchema(fixture.schema) {
		t.Fatal("foreign context issuer fixture")
	}
	if _, exactForeignOK := foreign.Exact(local.Result()); exactForeignOK {
		t.Fatal("foreign Reference crossed the context-value owner fence")
	}
	if fixture.schema.Equal(exact, foreign.Bottom()) || fixture.schema.LessOrEq(exact, foreign.Top()) {
		t.Fatal("foreign context value crossed lattice ownership")
	}
}

// TestContextValueRejectsDifferentAllocationRowsBeforeJoin prevents a
// tempting compensation: a context cell cannot silently become a map over
// unrelated Heap roots merely because the caller joined two values.
func TestContextValueRejectsDifferentAllocationRowsBeforeJoin(t *testing.T) {
	fixture := contextFixture(t)
	firstKey, firstKeyOK := fixture.heap.AllocationKeyAt(0)
	secondKey, secondKeyOK := fixture.schema.FreshAt(0)
	first, firstOK := fixture.schema.Local(firstKey, fixture.left, materialization.Recent)
	second, secondOK := fixture.schema.Local(secondKey, fixture.left, materialization.Recent)
	left, leftOK := fixture.schema.Exact(first.Result())
	right, rightOK := fixture.schema.Exact(second.Result())
	if !firstKeyOK || !secondKeyOK || !firstOK || !secondOK || !leftOK || !rightOK {
		t.Fatal("different-key context fixture")
	}
	if _, joinedOK := fixture.schema.Join(left, right); joinedOK {
		t.Fatal("context Join accepted rows from different allocation keys")
	}
}

func TestContextValueDomainExposesFiniteWideningWitness(t *testing.T) {
	fixture := contextFixture(t)
	key, keyOK := fixture.heap.AllocationKeyAt(0)
	local, localOK := fixture.schema.Local(key, fixture.left, materialization.Recent)
	value, valueOK := fixture.schema.Exact(local.Result())
	if !keyOK || !localOK || !valueOK {
		t.Fatal("widening fixture")
	}
	rank, rankOK := fixture.schema.WidenRank(value)
	if !rankOK || rank == 0 {
		t.Fatal("exact context value has no finite widening rank")
	}
	if topRank, topOK := fixture.schema.WidenRank(fixture.schema.Top()); topOK || topRank != 0 {
		t.Fatal("Top retained a widening witness")
	}
}
