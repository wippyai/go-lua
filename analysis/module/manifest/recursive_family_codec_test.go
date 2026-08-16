package manifest

import (
	"bytes"
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
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

func TestManifestRoundTripPreservesSharedRecursiveFamily(t *testing.T) {
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(typetable.NewRecord().Field("right", typeexpr.Optional(right)).Build())
	right.SetBody(typetable.NewRecord().Field("left", typeexpr.Optional(left)).Build())
	root := typetable.NewRecord().
		Field("left", left).
		Field("leftAgain", left).
		Field("right", right).
		Field("rightAgain", right).
		Build()

	wire, err := encodeType(root)
	if err != nil {
		t.Fatalf("encodeType: %v", err)
	}
	decoded, err := decodeType(wire)
	if err != nil {
		t.Fatalf("decodeType: %v", err)
	}
	record, ok := decoded.(*typ.Record)
	if !ok {
		t.Fatalf("decoded root = %T, want *typ.Record", decoded)
	}
	fields := make(map[string]typ.Type, len(record.Fields))
	for _, field := range record.Fields {
		fields[field.Name] = field.Type
	}
	if fields["left"] != fields["leftAgain"] {
		t.Fatal("shared Left recursive node lost identity across sibling fields")
	}
	if fields["right"] != fields["rightAgain"] {
		t.Fatal("shared Right recursive node lost identity across sibling fields")
	}
	decodedLeft, ok := fields["left"].(*typ.Recursive)
	if !ok {
		t.Fatalf("decoded Left = %T, want *typ.Recursive", fields["left"])
	}
	decodedRight, ok := fields["right"].(*typ.Recursive)
	if !ok {
		t.Fatalf("decoded Right = %T, want *typ.Recursive", fields["right"])
	}
	leftBody := decodedLeft.Body.(*typ.Record)
	rightBody := decodedRight.Body.(*typ.Record)
	leftRight := leftBody.Fields[0].Type.(*typ.Optional).Inner
	rightLeft := rightBody.Fields[0].Type.(*typ.Optional).Inner
	if leftRight != decodedRight || rightLeft != decodedLeft {
		t.Fatal("mutually recursive edges do not point at the shared decoded family")
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

func TestDecodeRejectsOutOfScopeRecursiveReference(t *testing.T) {
	_, err := decodeType(&typeWire{Kind: "recursiveRef", Binder: 1})
	if err == nil {
		t.Fatal("Decode accepted an out-of-scope recursive reference")
	}
}

func TestDecodeRejectsDuplicateRecursiveBinder(t *testing.T) {
	wire := &typeWire{
		Kind: "record",
		Fields: []fieldWire{
			{Name: "first", Type: &typeWire{Kind: "recursive", Binder: 1, Name: "First", Body: &typeWire{Kind: "string"}}},
			{Name: "second", Type: &typeWire{Kind: "recursive", Binder: 1, Name: "Second", Body: &typeWire{Kind: "number"}}},
		},
	}
	_, err := decodeType(wire)
	if err == nil {
		t.Fatal("Decode accepted a duplicate recursive binder")
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
