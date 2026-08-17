package target

import "testing"

func TestModelActivationCallbackReferencesResolveToOwnedIDs(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{callbackOwnerOperation("activation-model")}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"activation-model"}})
	if !ok {
		t.Fatal("activation owner operation missing")
	}
	callback, ok := contract.CallbackAt(op, 0)
	if !ok || callback == 0 {
		t.Fatalf("CallbackAt = %d/%v", callback, ok)
	}
	if got, ok := contract.CallbackOwner(callback); !ok || got != op {
		t.Fatalf("callback owner = %d/%v, want %d/true", got, ok, op)
	}
	if _, ok := contract.CallbackAt(op, -1); ok {
		t.Fatal("negative callback coordinate resolved")
	}
}
