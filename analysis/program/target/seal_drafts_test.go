package target

import "testing"

func TestSealDraftsCanonicalizeIndependentOperationAuthoring(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		builtin("draft-b", testBoolean, RowSpec{Tail: RowClosed}),
		builtin("draft-a", testString, RowSpec{Tail: RowClosed}),
	}})
	left, leftOK := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"draft-a"}})
	right, rightOK := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"draft-b"}})
	if !leftOK || !rightOK || left >= right {
		t.Fatalf("draft canonical order = %d/%v before %d/%v", left, leftOK, right, rightOK)
	}
	if contract.OperationCount() != 3 {
		t.Fatalf("draft operation count = %d, want two bound plus opaque", contract.OperationCount())
	}
}
