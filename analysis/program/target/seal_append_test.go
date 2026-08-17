package target

import "testing"

func TestSealAppendPublishesDenseOperationRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("append-relations", testString, RowSpec{Tail: RowClosed})}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"append-relations"}})
	if !ok {
		t.Fatal("appended operation binding missing")
	}
	input, ok := contract.Input(op)
	if !ok || input == 0 || contract.ValuesCount(input) != 1 {
		t.Fatalf("appended input = %d/%v count=%d", input, ok, contract.ValuesCount(input))
	}
	if got := contract.OutcomeCount(op); got != 1 {
		t.Fatalf("appended outcome count = %d, want 1", got)
	}
	if got := contract.BindingCount(op); got != 1 {
		t.Fatalf("appended binding count = %d, want 1", got)
	}
}
