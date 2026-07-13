package signature

import "testing"

func TestIntrinsicVocabularyIsSealed(t *testing.T) {
	if IntrinsicNone.Valid() {
		t.Fatal("zero intrinsic is valid")
	}
	if !IntrinsicLuaType.Valid() {
		t.Fatal("Lua type intrinsic is not valid")
	}
	if Intrinsic(255).Valid() {
		t.Fatal("unknown intrinsic is valid")
	}
}
