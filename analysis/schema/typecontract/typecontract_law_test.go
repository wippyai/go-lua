package typecontract

import (
	"bytes"
	"testing"
)

func TestPortableTypeContractLaws(t *testing.T) {
	t.Run("constructors reject unavailable values", func(t *testing.T) {
		if value, ok := NewEncoded(nil, 0); ok || value.Available() {
			t.Fatal("NewEncoded(nil) admitted an unavailable declaration")
		}
		if value, ok := NewEncoded([]byte{}, 0); ok || value.Available() {
			t.Fatal("NewEncoded(empty) admitted an unavailable declaration")
		}
		if value, ok := NewPrimitive(PrimitiveInvalid); ok || value.Available() {
			t.Fatal("NewPrimitive(PrimitiveInvalid) admitted an unavailable declaration")
		}
	})

	t.Run("encoded bytes are ownership isolated", func(t *testing.T) {
		input := []byte{0x10, 0x20, 0x30}
		value, ok := NewEncoded(input, 0)
		if !ok {
			t.Fatal("NewEncoded rejected valid input")
		}
		input[0] = 0xff
		if got := value.Bytes(); !bytes.Equal(got, []byte{0x10, 0x20, 0x30}) {
			t.Fatalf("constructor retained caller bytes: %x", got)
		}
		output := value.Bytes()
		output[1] = 0xee
		if got := value.Bytes(); !bytes.Equal(got, []byte{0x10, 0x20, 0x30}) {
			t.Fatalf("Bytes exposed mutable storage: %x", got)
		}
	})

	t.Run("formal scope is part of identity", func(t *testing.T) {
		left, leftOK := NewEncoded([]byte{0x41}, 1)
		right, rightOK := NewEncoded([]byte{0x41}, 2)
		if !leftOK || !rightOK || left.Equal(right) || left.Digest() == right.Digest() {
			t.Fatal("formal scope did not change declaration identity")
		}
	})

	t.Run("primitive and encoded representations cannot alias", func(t *testing.T) {
		primitive, primitiveOK := NewPrimitive(PrimitiveAny)
		encoded, encodedOK := NewEncoded([]byte{byte(PrimitiveAny)}, 0)
		if !primitiveOK || !encodedOK || primitive.Equal(encoded) || primitive.Digest() == encoded.Digest() {
			t.Fatal("primitive and encoded representations aliased")
		}
	})

	t.Run("equal declarations have stable digest", func(t *testing.T) {
		left, leftOK := NewEncoded([]byte{0x01, 0x02}, 3)
		right, rightOK := NewEncoded([]byte{0x01, 0x02}, 3)
		if !leftOK || !rightOK || !left.Equal(right) || left.Digest() != right.Digest() {
			t.Fatal("equal declarations did not share stable identity")
		}
		if left.Digest() != left.Digest() {
			t.Fatal("digest was not stable across reads")
		}
	})
}
