package target

import "testing"

func TestSubedgeFreezeResolvesAuthoredEdgesToDenseIDs(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("freeze-edge", false, false, false)}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"freeze-edge"}})
	if !ok || contract.SubedgeCount(op) == 0 {
		t.Fatalf("subedge owner = %d/%v count=%d", op, ok, contract.SubedgeCount(op))
	}
	edge, ok := contract.SubedgeAt(op, 0)
	if !ok || edge == 0 {
		t.Fatalf("SubedgeAt = %d/%v", edge, ok)
	}
	if owner, ok := contract.SubedgeOwner(edge); !ok || owner != op {
		t.Fatalf("SubedgeOwner = %d/%v, want %d/true", owner, ok, op)
	}
}
