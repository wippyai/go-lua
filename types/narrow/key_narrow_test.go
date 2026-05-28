package narrow_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestByTypeKey_Builtin(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number, typ.Nil)
	key := narrow.BuiltinTypeKey("number")

	result := narrow.ByTypeKey(base, key, nil)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number, got %v", result)
	}
}

func TestByTypeKey_ZeroKey(t *testing.T) {
	base := typ.String
	key := narrow.TypeKey{}

	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Fatal("zero key should return original type")
	}
}

func TestExcludeByTypeKey_Builtin(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.BuiltinTypeKey("string")

	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number, got %v", result)
	}
}

func TestByTypeKey_Nil(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ByTypeKey(nil, key, nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestByTypeKey_UnknownBuiltin(t *testing.T) {
	base := typ.String
	key := narrow.BuiltinTypeKey("nonexistent")
	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Error("unknown builtin kind should return original")
	}
}

func TestByTypeKey_HashWithoutResolver(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Error("hash key without resolver should return original")
	}
}

func TestByTypeKey_HashWithNilResolution(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	resolver := func(narrow.TypeKey) typ.Type { return nil }
	result := narrow.ByTypeKey(base, key, resolver)
	if result != base {
		t.Error("hash key with nil resolution should return original")
	}
}

func TestByTypeKey_HashWithResolver(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ByTypeKey(base, key, resolver)
	if !typ.TypeEquals(result, typ.String) {
		t.Errorf("expected string, got %v", result)
	}
}

func TestExcludeByTypeKey_Nil(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ExcludeByTypeKey(nil, key, nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestExcludeByTypeKey_ZeroKey(t *testing.T) {
	base := typ.String
	key := narrow.TypeKey{}
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("zero key should return original")
	}
}

func TestExcludeByTypeKey_UnknownBuiltin(t *testing.T) {
	base := typ.String
	key := narrow.BuiltinTypeKey("nonexistent")
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("unknown builtin kind should return original")
	}
}

func TestExcludeByTypeKey_BuiltinFullExclusionIsNever(t *testing.T) {
	// Excluding the kind that wholly covers the input proves the asserting
	// control path unreachable; the sound result is Never.
	base := typ.String
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if !typ.IsNever(result) {
		t.Errorf("excluding string from string should be never, got %v", result)
	}
}

func TestExcludeByTypeKey_LiteralFullExclusionIsNever(t *testing.T) {
	// A string literal is wholly a string, so type(x) ~= "string" is a
	// contradiction on that path.
	base := typ.LiteralString("merge")
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if !typ.IsNever(result) {
		t.Errorf("excluding string from \"merge\" should be never, got %v", result)
	}
}

func TestExcludeByTypeKey_BuiltinPreservesPlaceholder(t *testing.T) {
	// any/unknown are not provably the excluded kind; exclusion must not narrow.
	for _, base := range []typ.Type{typ.Any, typ.Unknown} {
		key := narrow.BuiltinTypeKey("string")
		result := narrow.ExcludeByTypeKey(base, key, nil)
		if typ.IsNever(result) {
			t.Errorf("excluding string from %v must not be never", base)
		}
	}
}

func TestExcludeByTypeKey_HashLiteralComplementInhabitedPreserved(t *testing.T) {
	// ExcludeType is overlap-based: string overlaps literal "merge" but its
	// complement is still inhabited, so a Never result is not a reachability
	// proof and the input is preserved.
	base := typ.String
	lit := typ.LiteralString("merge")
	key := narrow.HashTypeKey(lit.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == lit.Hash() {
			return lit
		}
		return nil
	}
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if !typ.TypeEquals(result, base) {
		t.Errorf("string excluding literal should stay string, got %v", result)
	}
}

func TestExcludeByTypeKey_HashWithoutResolver(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("hash key without resolver should return original")
	}
}

func TestExcludeByTypeKey_HashWithNilResolution(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	resolver := func(narrow.TypeKey) typ.Type { return nil }
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if result != base {
		t.Error("hash key with nil resolution should return original")
	}
}

func TestExcludeByTypeKey_HashWithResolver(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if !typ.TypeEquals(result, typ.Number) {
		t.Errorf("expected number, got %v", result)
	}
}

func TestExcludeByTypeKey_HashResultsNever(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if result != base {
		t.Error("exclusion resulting in never should return original")
	}
}

func TestBuiltinTypeKey_Empty(t *testing.T) {
	key := narrow.BuiltinTypeKey("")
	if !key.IsZero() {
		t.Error("empty name should create zero key")
	}
}

func TestHashTypeKey_Zero(t *testing.T) {
	key := narrow.HashTypeKey(0)
	if !key.IsZero() {
		t.Error("zero hash should create zero key")
	}
}

func TestTypeKey_Hash64(t *testing.T) {
	key1 := narrow.BuiltinTypeKey("string")
	key2 := narrow.BuiltinTypeKey("number")
	key3 := narrow.HashTypeKey(12345)

	if key1.Hash64() == key2.Hash64() {
		t.Error("different builtin keys should have different hashes")
	}
	if key1.Hash64() == key3.Hash64() {
		t.Error("builtin and hash keys should have different hashes")
	}
	if key1.Hash64() == 0 {
		t.Error("builtin key should have non-zero hash")
	}
}

func TestTypeKey_Hash64_Zero(t *testing.T) {
	key := narrow.TypeKey{}
	if key.Hash64() != 0 {
		t.Error("zero key should have zero hash")
	}
}

func TestTypeKey_Equal(t *testing.T) {
	key1 := narrow.BuiltinTypeKey("string")
	key2 := narrow.BuiltinTypeKey("string")
	key3 := narrow.BuiltinTypeKey("number")
	key4 := narrow.HashTypeKey(12345)

	if !key1.Equal(key2) {
		t.Error("same builtin keys should be equal")
	}
	if key1.Equal(key3) {
		t.Error("different builtin keys should not be equal")
	}
	if key1.Equal(key4) {
		t.Error("builtin and hash keys should not be equal")
	}
}
