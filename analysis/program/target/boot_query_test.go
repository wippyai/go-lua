package target

import "testing"

func TestBootQueryExposesOnlySealedCanonicalRows(t *testing.T) {
	contract := mustSeal(t, completeBootSpec("Lua 5.3", InitialMutable))
	if got := contract.InitialRootCount(); got != 2 {
		t.Fatalf("InitialRootCount = %d, want 2", got)
	}
	root, ok := contract.GlobalEnvRoot()
	if !ok || root == 0 {
		t.Fatalf("GlobalEnvRoot = %d/%v", root, ok)
	}
	shape, ok := contract.InitialRootBootShape(root)
	if !ok || shape == 0 {
		t.Fatalf("InitialRootBootShape = %d/%v", shape, ok)
	}
	if aggregate, ok := contract.BootShapeAggregate(shape); !ok || aggregate != BootAggregateTable {
		t.Fatalf("BootShapeAggregate = %d/%v", aggregate, ok)
	}
	if _, ok := contract.InitialRootAt(-1); ok {
		t.Fatal("negative root query resolved")
	}
	if _, ok := contract.InitialRootAt(contract.InitialRootCount()); ok {
		t.Fatal("out-of-range root query resolved")
	}
}
