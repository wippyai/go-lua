package target

import "testing"

func TestGsubTableBranchIsAbsentOutsideItsClosedOwner(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{builtin("ordinary", testString, RowSpec{Tail: RowClosed})}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"ordinary"}})
	if !ok {
		t.Fatal("ordinary operation missing")
	}
	if _, key, _, _, _, ok := contract.GsubTableReplacement(op); ok || key != GsubTableKeyInvalid {
		t.Fatalf("ordinary operation acquired gsub branch: key=%d ok=%v", key, ok)
	}
	if contract.GsubTableReplacementEffectAliasCount(op) != 0 {
		t.Fatal("ordinary operation exposed gsub effect aliases")
	}
}
