package target

import "testing"

func TestGsubTableSealRejectsNonGsubOwnersBeforeRouteInspection(t *testing.T) {
	draft := operationDraft{bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"ordinary"}}}}
	if _, err := draft.freezeGsubTableReplacement(GsubTableReplacementSpec{}); err == nil {
		t.Fatal("non-gsub owner accepted a table replacement branch")
	}
	stringDraft := operationDraft{bindings: []BindingSpec{{Namespace: BindingModule, Owner: []string{"string"}, Member: []string{"gsub"}}}}
	if _, err := stringDraft.freezeGsubTableReplacement(GsubTableReplacementSpec{}); err == nil {
		t.Fatal("incomplete gsub replacement accepted")
	}
}
