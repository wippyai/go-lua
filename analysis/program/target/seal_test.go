package target

import "testing"

func TestSealConsumesAuthoringSpecOnEveryFirstAttempt(t *testing.T) {
	spec := Spec{Operations: []OperationSpec{builtin("seal-once", testString, RowSpec{Tail: RowClosed})}}
	if _, err := testSeal(&spec); err != nil {
		t.Fatal(err)
	}
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("successful Seal left the authoring spec reusable")
	}
	bad := Spec{Operations: []OperationSpec{{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"bad"}}}, Input: ValuesSpec{Tail: ValuesUnknown}}}}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("invalid spec unexpectedly sealed")
	}
	bad.Operations = []OperationSpec{builtin("replacement", testString, RowSpec{Tail: RowClosed})}
	if _, err := testSeal(&bad); err == nil {
		t.Fatal("failed Seal left the authoring spec reusable")
	}
}
