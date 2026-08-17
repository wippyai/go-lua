package wire

import (
	"bytes"
	"testing"

	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

func TestManifestRoundTripRecursiveExport(t *testing.T) {
	reader := typ.NewRecursive("StateReader", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("id", typ.String).
			Field("parent", typeexpr.Optional(self)).
			Build()
	})
	m := New("keeper/state/reader")
	m.SetExport(reader)

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Contains(data, []byte(`"kind": "recursive"`)) ||
		!bytes.Contains(data, []byte(`"kind": "recursiveRef"`)) {
		t.Fatalf("recursive family is not explicit in wire:\n%s", data)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !typ.TypeEquals(decoded.Export, reader) {
		t.Fatalf("decoded export = %v, want %v", decoded.Export, reader)
	}
}

func TestManifestRoundTripMutuallyRecursiveTypeFamily(t *testing.T) {
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(typetable.NewRecord().Field("right", typeexpr.Optional(right)).Build())
	right.SetBody(typetable.NewRecord().Field("left", typeexpr.Optional(left)).Build())

	m := New("mutual")
	m.DefineType("Left", left)
	m.SetExport(right)
	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !typ.TypeEquals(decoded.Types["Left"], left) {
		t.Fatalf("decoded Left = %v, want %v", decoded.Types["Left"], left)
	}
	if !typ.TypeEquals(decoded.Export, right) {
		t.Fatalf("decoded export = %v, want %v", decoded.Export, right)
	}
}

func TestRecursiveManifestBytesAreDeterministic(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	m := New("recursive/deterministic")
	m.SetExport(typetable.NewRecord().Field("a", node).Field("b", node).Build())

	first, err := Encode(m)
	if err != nil {
		t.Fatalf("first Encode: %v", err)
	}
	second, err := Encode(m)
	if err != nil {
		t.Fatalf("second Encode: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("recursive manifest bytes changed across encode:\n%s\n%s", first, second)
	}
}

func TestDecodeLegacyManifestWithoutRecursiveWire(t *testing.T) {
	legacy := []byte(`{"path":"legacy","export":{"kind":"array","element":{"kind":"string"}}}`)
	decoded, err := Decode(legacy)
	if err != nil {
		t.Fatalf("Decode legacy manifest: %v", err)
	}
	if !typ.TypeEquals(decoded.Export, typ.NewArray(typ.String)) {
		t.Fatalf("decoded legacy export = %v, want string[]", decoded.Export)
	}
}
