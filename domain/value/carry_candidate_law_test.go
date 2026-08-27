package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/carrier"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestEveryValueCarryTransformCandidateIsARelationSubject is the binding
// precondition Value owes every rule that carries with one of its transforms.
// A rule draws its candidates from one relation, and the plan admits its carry
// only when the transform's candidate carrier is that relation's subject. A
// transform whose candidate no relation subjects therefore cannot be bound by
// any rule, however correct the transform itself is.
//
// Value states this by publishing its two constructor-receipt directories,
// rather than by moving the transforms onto the coordinate the way Heap did.
// The difference is which package owns the descriptor: Heap's constructor
// descriptors live beside the fold in a package the heap axis cannot import,
// so no heap relation could ever have rows of them, while AllocationResult and
// FreshResultCall are declared by the value package itself and their
// directories are Value's own sealed order.
func TestEveryValueCarryTransformCandidateIsARelationSubject(t *testing.T) {
	catalog := valuedomain.AxisMemberCatalog()
	if len(catalog.CarryTransforms) == 0 {
		t.Fatal("value declares no carry transform")
	}
	subjects := make(map[carrier.Key]struct{}, len(catalog.Relations))
	for _, relation := range catalog.Relations {
		subjects[relation.Subject] = struct{}{}
	}
	for _, transform := range catalog.CarryTransforms {
		if _, ok := subjects[transform.Candidate]; !ok {
			t.Fatalf("carry transform %q carries candidate %q, which is the subject of no value relation", transform.Key, transform.Candidate)
		}
	}
}

// TestEveryValueCandidateDirectoryIsDenseAndInvertible states what a published
// directory owes: a census, a row at every ordinal below it, and an ordinal
// that takes each row back to exactly the position it was read from. Without
// the inverse, a rule that resolves a candidate cannot address the row it
// resolved, and the directory is only a list.
func TestEveryValueCandidateDirectoryIsDenseAndInvertible(t *testing.T) {
	// The subject seals both directory kinds: a table constructor is a Program
	// allocation, and a host call with a Target fresh result is an admitted
	// fresh-result row.
	const carryCandidateSource = "local t = {}\nlocal co = coroutine.create(function() end)\nreturn t, co\n"
	_, schema := sealValueSource(t, "carry_candidate.lua", carryCandidateSource)

	t.Run("allocation", func(t *testing.T) {
		count := schema.AllocationResultCount()
		if count == 0 {
			t.Skip("fixture seals no Program allocation")
		}
		for index := 0; index < count; index++ {
			result, resultOK := schema.AllocationResultAt(index)
			if !resultOK {
				t.Fatalf("AllocationResultAt(%d)", index)
			}
			ordinal, ordinalOK := schema.AllocationResultOrdinal(result)
			if !ordinalOK || int(ordinal) != index {
				t.Fatalf("AllocationResultOrdinal at %d = %d/%t", index, ordinal, ordinalOK)
			}
			id, idOK := schema.AllocationResultIDAt(index)
			if !idOK || !id.Available() {
				t.Fatalf("AllocationResultIDAt(%d)", index)
			}
			if _, coordinateOK := result.Coordinate(); !coordinateOK {
				t.Fatalf("allocation %d publishes no destination coordinate", index)
			}
		}
		if _, ok := schema.AllocationResultAt(count); ok {
			t.Fatal("the allocation directory answered past its own census")
		}
	})

	t.Run("fresh-result", func(t *testing.T) {
		count := schema.FreshResultCallCount()
		if count == 0 {
			t.Skip("fixture seals no admitted fresh result")
		}
		for index := 0; index < count; index++ {
			row, rowOK := schema.FreshResultCallAt(index)
			if !rowOK {
				t.Fatalf("FreshResultCallAt(%d)", index)
			}
			ordinal, ordinalOK := schema.FreshResultCallOrdinal(row)
			if !ordinalOK || int(ordinal) != index {
				t.Fatalf("FreshResultCallOrdinal at %d = %d/%t", index, ordinal, ordinalOK)
			}
			id, idOK := schema.FreshResultCallIDAt(index)
			if !idOK || !id.Available() {
				t.Fatalf("FreshResultCallIDAt(%d)", index)
			}
			resolved, resolvedOK := schema.FreshResultCallForID(id)
			if !resolvedOK || resolved != row {
				t.Fatalf("FreshResultCallForID at %d did not answer the row it identifies", index)
			}
			if _, coordinateOK := row.Coordinate(); !coordinateOK {
				t.Fatalf("fresh result %d publishes no destination coordinate", index)
			}
		}
		if _, ok := schema.FreshResultCallAt(count); ok {
			t.Fatal("the fresh-result directory answered past its own census")
		}
	})
}
