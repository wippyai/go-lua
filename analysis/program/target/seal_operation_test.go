package target

import "testing"

func TestSealOperationRequiresCompleteOutcomeAuthority(t *testing.T) {
	valid := Spec{Operations: []OperationSpec{builtin("complete", testString, RowSpec{Tail: RowClosed})}}
	if _, err := testSeal(&valid); err != nil {
		t.Fatal(err)
	}
	invalid := Spec{Operations: []OperationSpec{{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"incomplete"}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
	}}}
	if _, err := testSeal(&invalid); err == nil {
		t.Fatal("operation without outcomes was accepted")
	}
}
