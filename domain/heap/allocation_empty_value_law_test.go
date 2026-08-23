package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

const emptyAllocationSource = `
local fresh = {}
local closed = { value = 1 }
return fresh, closed
`

// TestEmptyAllocationFactConstructsTheFreshObjectItsCandidateDenotes states
// what the empty constructor's fold decides: over the world its predecessor
// read holds, it concludes that world with a fresh mutable object allocated at
// the candidate coordinate, containing nothing, and eligible exactly when the
// sealed allocation is a table. The shape is the observable distinction the
// fold makes, so the law states it in the schema's own vocabulary rather than
// asserting a lattice relation the constructor does not have: allocating at a
// coordinate replaces the world there, it does not extend it.
func TestEmptyAllocationFactConstructsTheFreshObjectItsCandidateDenotes(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "empty_allocation_fact", emptyAllocationSource, nil)
	candidates := schema.EmptyAllocationCount()
	if candidates == 0 {
		t.Fatal("fixture sealed no empty allocation candidate")
	}
	for index := 0; index < candidates; index++ {
		key, keyOK := schema.EmptyAllocationAt(index)
		if !keyOK {
			t.Fatalf("EmptyAllocationAt(%d)", index)
		}
		predecessor, predecessorOK := schema.EmptyObject(key)
		if !predecessorOK {
			t.Fatalf("predecessor world at ordinal %d", index)
		}
		next, outcome := heapdomain.EmptyAllocationFact(key, predecessor)
		if outcome != structure.Concrete {
			t.Fatalf("ordinal %d concluded %v, want the constructed world", index, outcome)
		}
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(key)
		if !originOK {
			t.Fatalf("sealed allocation origin at ordinal %d", index)
		}
		shape := heapdomain.ShapeIneligible
		if kind == heapdomain.AllocationTable {
			shape = heapdomain.ShapeEligible
		}
		none, noneOK := schema.ContainmentNone()
		initializer, initOK := schema.BeginObject(shape, heapdomain.FrozenMutable, none)
		fresh, freshOK := initializer.Finish()
		want, wantOK := schema.Create(predecessor, key, fresh)
		if !noneOK || !initOK || !freshOK || !wantOK {
			t.Fatalf("constructed world at ordinal %d", index)
		}
		if !schema.Domain().Equal(next, want) {
			t.Fatalf("ordinal %d concluded a world other than the fresh %v object its candidate denotes", index, shape)
		}
		again, againOutcome := heapdomain.EmptyAllocationFact(key, predecessor)
		if againOutcome != outcome || !schema.Domain().Equal(again, next) {
			t.Fatalf("ordinal %d concluded two different worlds from one predecessor", index)
		}
	}
}

// TestEmptyAllocationFactRefusesWhatItsDirectoryDoesNotContain states the
// other half: the fold decides only for the coordinates the empty directory
// publishes, and only over a world its predecessor actually holds. A Bottom
// predecessor is no world at all, and a coordinate of another constructor
// form, of another root kind, or of another schema is not this fold's.
func TestEmptyAllocationFactRefusesWhatItsDirectoryDoesNotContain(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "empty_allocation_fact_refusal", emptyAllocationSource, nil)
	if schema.EmptyAllocationCount() == 0 || schema.ClosedAllocationCount() == 0 {
		t.Fatalf("fixture sealed empty=%d closed=%d, want both forms", schema.EmptyAllocationCount(), schema.ClosedAllocationCount())
	}
	empty, emptyOK := schema.EmptyAllocationAt(0)
	if !emptyOK {
		t.Fatal("empty allocation candidate")
	}
	if _, outcome := heapdomain.EmptyAllocationFact(empty, schema.Bottom()); outcome != structure.Refuse {
		t.Fatalf("a Bottom predecessor concluded %v", outcome)
	}
	closed, closedOK := schema.ClosedAllocationAt(0)
	if !closedOK {
		t.Fatal("closed allocation candidate")
	}
	world, worldOK := schema.EmptyObject(empty)
	if !worldOK {
		t.Fatal("predecessor world")
	}
	if _, outcome := heapdomain.EmptyAllocationFact(closed, world); outcome != structure.Refuse {
		t.Fatalf("a closed constructor coordinate concluded %v", outcome)
	}
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() == heapdomain.RootAllocation {
			continue
		}
		if _, outcome := heapdomain.EmptyAllocationFact(key, world); outcome != structure.Refuse {
			t.Fatalf("a non-allocation root concluded %v", outcome)
		}
	}
	var unowned heapdomain.Key
	if _, outcome := heapdomain.EmptyAllocationFact(unowned, world); outcome != structure.Refuse {
		t.Fatalf("an unowned coordinate concluded %v", outcome)
	}
	_, foreign, _ := compactHeapFixture(t, "empty_allocation_fact_foreign", emptyAllocationSource, nil)
	foreignEmpty, foreignEmptyOK := foreign.EmptyAllocationAt(0)
	if !foreignEmptyOK {
		t.Fatal("foreign empty allocation candidate")
	}
	if _, outcome := heapdomain.EmptyAllocationFact(foreignEmpty, world); outcome != structure.Refuse {
		t.Fatalf("a foreign coordinate concluded %v over a local world", outcome)
	}
}
