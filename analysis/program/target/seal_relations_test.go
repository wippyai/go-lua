package target

import "testing"

func TestSealRelationsRejectDuplicateBindingsAndCanonicalizeOrder(t *testing.T) {
	bindings, err := freezeBindings([]BindingSpec{
		{Namespace: BindingBuiltin, Member: []string{"z"}},
		{Namespace: BindingBuiltin, Member: []string{"a"}},
	})
	if err != nil || len(bindings) != 2 || bindings[0].Member[0] != "a" {
		t.Fatalf("freezeBindings = %#v/%v", bindings, err)
	}
	if _, err := freezeBindings([]BindingSpec{
		{Namespace: BindingBuiltin, Member: []string{"same"}},
		{Namespace: BindingBuiltin, Member: []string{"same"}},
	}); err == nil {
		t.Fatal("duplicate bindings were accepted")
	}
}
