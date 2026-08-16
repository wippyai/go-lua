package typ

import "testing"

func TestCloneFunctionCopiesMutableSlicesWithoutRebuildingSemantics(t *testing.T) {
	param := NewTypeParam("T", String)
	original := Func().
		TypeParamRef(param).
		Param("value", String).
		Returns(Boolean).
		Build()

	_ = original.String()
	clone := CloneFunction(original)

	if clone == nil {
		t.Fatal("CloneFunction returned nil")
	}
	if clone == original {
		t.Fatal("CloneFunction returned the original pointer")
	}
	if clone.Hash() != original.Hash() || !TypeEquals(clone, original) {
		t.Fatalf("clone changed function semantics: got %s hash %d, want %s hash %d", clone, clone.Hash(), original, original.Hash())
	}

	clone.TypeParams[0] = NewTypeParam("U", Number)
	clone.Params[0].Type = Number
	clone.Returns[0] = String

	if original.TypeParams[0] != param {
		t.Fatalf("clone mutation changed original type param")
	}
	if original.Params[0].Type != String {
		t.Fatalf("clone mutation changed original param type: %v", original.Params[0].Type)
	}
	if original.Returns[0] != Boolean {
		t.Fatalf("clone mutation changed original return type: %v", original.Returns[0])
	}
}

func TestCloneFunctionNil(t *testing.T) {
	if CloneFunction(nil) != nil {
		t.Fatal("CloneFunction(nil) must return nil")
	}
}

func TestCloneFunctionDoesNotShareEqualityHashCache(t *testing.T) {
	recursive := NewRecursive("R", func(self Type) Type { return NewArray(self) })
	source := Func().Returns(recursive).Build()
	sourceHash := EqualityHash(source)
	clone := CloneFunction(source)
	clone.Returns[0] = Boolean
	if got := EqualityHash(clone); got == sourceHash {
		t.Fatalf("cloned function reused source equality cache: got=%d source=%d", got, sourceHash)
	}
	if got := EqualityHash(source); got != sourceHash {
		t.Fatalf("mutating cloned function changed source equality hash: got=%d want=%d", got, sourceHash)
	}
}
