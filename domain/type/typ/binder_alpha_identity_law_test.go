package typ

import (
	"context"
	"testing"
)

// declaredChannel builds one generic declaration whose body never mentions its
// own parameter. It is the shape that makes formal identity observable: every
// distinction between two applications lives in the type argument alone.
func declaredChannel() *Generic {
	return NewGeneric("Channel", []*TypeParam{NewTypeParam("T", nil)}, NewInterface("Channel", nil))
}

func decodeAtShiftedPosition(t *testing.T, value Type, padding int) Type {
	t.Helper()
	payload := value
	for i := 0; i < padding; i++ {
		payload = NewTuple(String, payload)
	}
	receipt, err := EncodeCanonicalFormals(context.Background(), payload, nil)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), receipt, nil)
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals: %v", err)
	}
	for i := 0; i < padding; i++ {
		tuple, ok := decoded.(*Tuple)
		if !ok || len(tuple.Elements) != 2 {
			t.Fatalf("padding %d unwrapped to %T", i, decoded)
		}
		decoded = tuple.Elements[1]
	}
	return decoded
}

// TestDecodedGenericIdentityIsIndependentOfGraphPosition states that a formal
// binds by (binder, ordinal). Two decodes of one declaration place its formal
// at different graph positions; the declaration is still one type.
func TestDecodedGenericIdentityIsIndependentOfGraphPosition(t *testing.T) {
	declaration := declaredChannel()

	flat := decodeAtShiftedPosition(t, declaration, 0)
	shifted := decodeAtShiftedPosition(t, declaration, 3)

	if !TypeEquals(flat, shifted) {
		t.Fatalf("two decodes of one generic declaration compare unequal: %s vs %s", flat, shifted)
	}
	if EqualityHash(flat) != EqualityHash(shifted) {
		t.Fatalf("two decodes of one generic declaration hash differently: %d vs %d", EqualityHash(flat), EqualityHash(shifted))
	}
}

// TestGenericIdentityIgnoresFormalSpelling states the same law for source
// construction: a formal's presentation name is not part of identity.
func TestGenericIdentityIgnoresFormalSpelling(t *testing.T) {
	spelledT := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)}, nil)
	paramU := NewTypeParam("U", nil)
	spelledU := NewGeneric("Box", []*TypeParam{paramU}, nil)
	spelledT.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "item", Type: spelledT.TypeParams[0]}}}))
	spelledU.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "item", Type: paramU}}}))

	if !TypeEquals(spelledT, spelledU) {
		t.Fatalf("alpha-equivalent generics compare unequal: %s vs %s", spelledT, spelledU)
	}
	if EqualityHash(spelledT) != EqualityHash(spelledU) {
		t.Fatalf("alpha-equivalent generics hash differently")
	}
}

// TestGenericIdentityDistinguishesFormalPosition states the converse: binding
// is positional, so a body that swaps two formals is a different declaration.
func TestGenericIdentityDistinguishesFormalPosition(t *testing.T) {
	first := NewTypeParam("A", nil)
	second := NewTypeParam("B", nil)
	straight := NewGeneric("Pair", []*TypeParam{first, second}, RebuildRecord(RecordParts{
		Fields: []Field{{Name: "left", Type: first}, {Name: "right", Type: second}},
	}))
	swappedFirst := NewTypeParam("A", nil)
	swappedSecond := NewTypeParam("B", nil)
	swapped := NewGeneric("Pair", []*TypeParam{swappedFirst, swappedSecond}, RebuildRecord(RecordParts{
		Fields: []Field{{Name: "left", Type: swappedSecond}, {Name: "right", Type: swappedFirst}},
	}))

	if TypeEquals(straight, swapped) {
		t.Fatalf("generics that bind their formals in opposite positions compare equal")
	}
}
