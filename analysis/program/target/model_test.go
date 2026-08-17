package target

import "testing"

func TestModelHandlesRemainDenseAndZeroInvalid(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("model", testString, RowSpec{Tail: RowClosed})}})
	if _, ok := contract.OperationAt(0); !ok {
		t.Fatal("sealed operation did not receive a model handle")
	}
	if _, ok := contract.Input(Operation(1)); !ok {
		t.Fatal("sealed operation did not receive an input Values handle")
	}
	if _, ok := contract.Input(0); ok {
		t.Fatal("zero Operation handle resolved")
	}
	if _, ok := contract.TypeDeclaration(0); ok {
		t.Fatal("zero Type handle resolved")
	}
}
