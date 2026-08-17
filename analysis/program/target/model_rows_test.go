package target

import "testing"

func TestModelRowsKeepOperationOwnedRangesCorrelated(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("row-a", testString, RowSpec{Tail: RowClosed}),
		builtin("row-b", testBoolean, RowSpec{Tail: RowClosed}),
	}})
	for _, name := range []string{"row-a", "row-b"} {
		op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
		if !ok || contract.OutcomeCount(op) != 1 || contract.EffectCount(op) != 0 {
			t.Fatalf("%s row shape = op:%d/%v outcomes:%d effects:%d", name, op, ok, contract.OutcomeCount(op), contract.EffectCount(op))
		}
	}
}
