package target

import "testing"

func TestOperationQueriesKeepBoundPrefixAndOpaqueLast(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("query-a", testString, RowSpec{Tail: RowClosed}),
		builtin("query-b", testString, RowSpec{Tail: RowClosed}),
	}})
	if got := contract.OperationCount(); got != 3 {
		t.Fatalf("OperationCount = %d, want bound operations plus opaque", got)
	}
	if got := contract.BoundOperationCount(); got != 2 {
		t.Fatalf("BoundOperationCount = %d, want 2", got)
	}
	op, ok := contract.OperationAt(contract.OperationCount() - 1)
	if !ok {
		t.Fatal("opaque operation missing")
	}
	if opaque, ok := contract.Opaque(); !ok || opaque != op {
		t.Fatalf("Opaque = %d/%v, want %d/true", opaque, ok, op)
	}
	if _, ok := contract.OperationAt(contract.OperationCount()); ok {
		t.Fatal("out-of-range operation resolved")
	}
}
