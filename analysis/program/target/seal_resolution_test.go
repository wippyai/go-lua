package target

import "testing"

func TestSealResolutionRetainsProducedOperationAnchors(t *testing.T) {
	contract := mustSeal(t, deltaProduced(0))
	parent, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"produced"}})
	if !ok || contract.ProducedCount(parent, 0) != 1 {
		t.Fatalf("produced resolution = op:%d/%v count:%d", parent, ok, contract.ProducedCount(parent, 0))
	}
	_, child, ok := contract.ProducedAt(parent, 0, 0)
	if !ok || child == 0 || child == parent {
		t.Fatalf("produced child = %d/%v, parent=%d", child, ok, parent)
	}
}
