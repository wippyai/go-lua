package heap_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// A Partition's validity is an obligation of the Object that carries it, and
// Object.Valid discharges it. Admits proves the Object first and then walks
// its coordinates - one pass per legal key kind, one per exception, one per
// present coordinate of each - so everything below that first proof reads it.
// Re-deriving it at those coordinates makes the fence cost the product of the
// Object's kinds, exceptions, and presents rather than its size, and
// admission runs at the frequency of heap reads and writes, so the product is
// paid on every evaluation.
//
// The law is relative to the Object's own proof, which is the honest bound:
// admitting an Object proves no more Partitions and derives no more residual
// defaults than proving that Object does.
func TestHeapAdmissionProvesNoMoreThanTheObjectItAdmits(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "admission_proof_once", compactHeapSource, nil)
	allocation := compactAllocationKeys(t, schema, 1)[0]
	slot, payload, selector := compactField(t, schema, allocation)
	none := compactNone(t, schema)
	state := compactPresent(t, schema, slot, payload, none, none)
	object := compactBuiltObject(t, schema, heapdomain.ShapeEligible, heapdomain.FrozenMutable, none, compactObjectStep{selector: selector, state: state})
	value := compactValue(t, schema, allocation, object)

	heapdomain.DbgHeapReset()
	if !object.Valid() {
		t.Fatal("owner-issued Object is not valid")
	}
	owed := heapdomain.DbgHeap()
	if owed.PartitionValidations != 1 {
		t.Fatalf("proving one Object proved its Partition %d times, want 1", owed.PartitionValidations)
	}

	heapdomain.DbgHeapReset()
	if !schema.Admits(allocation, value) {
		t.Fatal("owner-issued value rejected by its own schema")
	}
	admission := heapdomain.DbgHeap()
	if admission.PartitionValidations != owed.PartitionValidations {
		t.Errorf("admitting one Object proved its Partition %d times against the %d its own proof owes: the proof is discharged where the Object is proved and read at every coordinate below it",
			admission.PartitionValidations, owed.PartitionValidations)
	}
	if admission.DefaultDerivations > owed.DefaultDerivations {
		t.Errorf("admission derived %d residual defaults against the %d its Object's proof owes: a baseline belongs to the exception it checks, not to every coordinate that mentions the partition",
			admission.DefaultDerivations, owed.DefaultDerivations)
	}
}
