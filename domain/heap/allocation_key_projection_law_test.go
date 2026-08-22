package heap

import "testing"

func TestAllocationKeyProjectionIsTheExactLeadingRootPrefix(t *testing.T) {
	schema := minimalFreezeSchema(t, "allocation-key-projection")
	schema.owner.roots = append(schema.owner.roots, rootRow{kind: RootBoot})

	if got, want := schema.KeyCount(), 3; got != want {
		t.Fatalf("Heap root count = %d, want %d", got, want)
	}
	if got, want := schema.AllocationKeyCount(), 2; got != want {
		t.Fatalf("allocation root count = %d, want %d", got, want)
	}
	for index := 0; index < schema.AllocationKeyCount(); index++ {
		key, keyOK := schema.AllocationKeyAt(index)
		dense, denseOK := schema.AllocationKeyIndex(key)
		if !keyOK || key.Kind() != RootAllocation || !denseOK || dense != index {
			t.Fatalf("allocation coordinate %d = kind=%v index=%d key=%t dense=%t", index, key.Kind(), dense, keyOK, denseOK)
		}
	}
	if _, ok := schema.AllocationKeyAt(schema.AllocationKeyCount()); ok {
		t.Fatal("allocation projection admitted the first Boot coordinate")
	}
	boot, bootOK := schema.KeyAt(2)
	if !bootOK || boot.Kind() != RootBoot {
		t.Fatal("Heap fixture did not retain its Boot coordinate")
	}
	if _, ok := schema.AllocationKeyIndex(boot); ok {
		t.Fatal("allocation inverse admitted a Boot root")
	}
}
