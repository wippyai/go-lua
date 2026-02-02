package narrow

import "testing"

func TestTypeKeyKind_Values(t *testing.T) {
	if TypeKeyInvalid != 0 {
		t.Errorf("TypeKeyInvalid = %d, want 0", TypeKeyInvalid)
	}
	if TypeKeyBuiltin != 1 {
		t.Errorf("TypeKeyBuiltin = %d, want 1", TypeKeyBuiltin)
	}
	if TypeKeyHash != 2 {
		t.Errorf("TypeKeyHash = %d, want 2", TypeKeyHash)
	}
}

func TestBuiltinTypeKey(t *testing.T) {
	k := BuiltinTypeKey("number")
	if k.Kind != TypeKeyBuiltin {
		t.Error("Kind should be TypeKeyBuiltin")
	}
	if k.Name != "number" {
		t.Errorf("Name = %q, want %q", k.Name, "number")
	}
}

func TestBuiltinTypeKey_Empty(t *testing.T) {
	k := BuiltinTypeKey("")
	if !k.IsZero() {
		t.Error("empty name should produce zero key")
	}
}

func TestHashTypeKey(t *testing.T) {
	k := HashTypeKey(12345)
	if k.Kind != TypeKeyHash {
		t.Error("Kind should be TypeKeyHash")
	}
	if k.Hash != 12345 {
		t.Errorf("Hash = %d, want 12345", k.Hash)
	}
}

func TestHashTypeKey_Zero(t *testing.T) {
	k := HashTypeKey(0)
	if !k.IsZero() {
		t.Error("zero hash should produce zero key")
	}
}

func TestTypeKey_IsZero(t *testing.T) {
	var k TypeKey
	if !k.IsZero() {
		t.Error("zero value should be zero")
	}
}

func TestTypeKey_Hash64_Builtin(t *testing.T) {
	k := BuiltinTypeKey("string")
	h := k.Hash64()
	if h == 0 {
		t.Error("Hash64 should not be zero for valid key")
	}
}

func TestTypeKey_Hash64_Hash(t *testing.T) {
	k := HashTypeKey(99999)
	h := k.Hash64()
	if h == 0 {
		t.Error("Hash64 should not be zero for valid key")
	}
}

func TestTypeKey_Hash64_Invalid(t *testing.T) {
	var k TypeKey
	h := k.Hash64()
	if h != 0 {
		t.Error("Hash64 should be zero for invalid key")
	}
}

func TestTypeKey_Equal(t *testing.T) {
	k1 := BuiltinTypeKey("number")
	k2 := BuiltinTypeKey("number")
	k3 := BuiltinTypeKey("string")

	if !k1.Equal(k2) {
		t.Error("same keys should be equal")
	}
	if k1.Equal(k3) {
		t.Error("different keys should not be equal")
	}
}

func TestTypeKey_Equal_DifferentKinds(t *testing.T) {
	k1 := BuiltinTypeKey("test")
	k2 := HashTypeKey(123)

	if k1.Equal(k2) {
		t.Error("different kinds should not be equal")
	}
}
