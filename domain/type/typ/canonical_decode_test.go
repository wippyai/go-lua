package typ

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/kind"
)

func TestDecodeCanonicalRoundTripsCompleteNonrecursiveCorpus(t *testing.T) {
	param := NewTypeParam("T", NewInterface("Comparable", nil))
	record := RebuildRecord(RecordParts{
		Fields: []Field{
			{Name: "a", Type: MaterializeOptional(Number), Optional: true},
			{Name: "z", Type: NewArray(String), Readonly: true},
		},
		StaticMembers: []StaticMember{
			{Kind: StaticMemberStringIndex, Name: "kind", Type: LiteralString("record"), Readonly: true},
			{Kind: StaticMemberIntIndex, Index: 7, Type: Boolean},
		},
		MapKey: String, MapValue: Unknown, Metatable: NewMeta(String), Open: true, AssumeSorted: true,
	})
	function := RebuildFunction(FunctionParts{
		TypeParams: []*TypeParam{param},
		Params: []Param{
			{Name: "presentation-only", Type: record},
			{Name: "self", Type: param, Optional: true, Receiver: true},
		},
		Variadic: Integer,
		Returns:  []Type{NewReadonlyMap(String, Boolean), param},
	})
	generic := NewGeneric("Box", []*TypeParam{param}, RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: param}}}))
	shared := NewArray(NewTuple(String, Number))
	corpus := []Type{
		nil, Nil, Boolean, Number, Integer, String, Any, Unknown, Never, Self,
		LiteralBool(false), LiteralBool(true), LiteralInt(-19), LiteralNumber(2.5), LiteralString("hello\x00world"),
		NewRef("module/path", "Thing"), MaterializeOptional(String),
		MaterializeUnion([]Type{String, Integer, LiteralString("x")}),
		MaterializeIntersection([]Type{record, NewInterface("Named", nil)}),
		NewArray(Number), NewMap(String, Integer), NewReadonlyMap(String, Integer),
		NewTuple(String, Number, MaterializeOptional(Boolean)), NewTuple(shared, shared),
		record, function, generic, Instantiate(generic, String), param,
		NewInterface("Reader", []Method{{Name: "read", Type: Func().Param("presentation", Number).Returns(String).Build()}}),
		NewMeta(record),
		NewAnnotated(function, []annotation.Annotation{{Name: "presentation", Arg: annotation.Int64Arg(3)}}),
		NewAlias("PresentationOnly", function),
	}
	for index, original := range corpus {
		encoded, err := EncodeCanonical(context.Background(), original)
		if err != nil {
			t.Fatalf("encode corpus[%d] %T: %v", index, original, err)
		}
		decoded, err := DecodeCanonical(context.Background(), encoded)
		if err != nil {
			t.Fatalf("decode corpus[%d] %T: %v", index, original, err)
		}
		if !TypeEquals(original, decoded) {
			t.Fatalf("corpus[%d] %T decoded to unequal %T", index, original, decoded)
		}
		roundTrip, err := EncodeCanonical(context.Background(), decoded)
		if err != nil || !bytes.Equal(encoded, roundTrip) {
			t.Fatalf("corpus[%d] bytes changed: %x / %x / %v", index, encoded, roundTrip, err)
		}
	}
}

func TestDecodeCanonicalRoundTripsRawIEEEFloatLiterals(t *testing.T) {
	for _, bits := range []uint64{
		0x0000000000000000,
		0x8000000000000000,
		0x7ff0000000000000,
		0xfff0000000000000,
		0x7ff8000000000001,
		0x7ff8000000000002,
		0x7ff0000000000001,
	} {
		original := LiteralNumber(math.Float64frombits(bits))
		encoded, err := EncodeCanonical(context.Background(), original)
		if err != nil {
			t.Fatalf("encode %#x: %v", bits, err)
		}
		decoded, err := DecodeCanonical(context.Background(), encoded)
		if err != nil {
			t.Fatalf("decode %#x: %v", bits, err)
		}
		literal, ok := decoded.(*Literal)
		if !ok || literal.Base() != kind.Number {
			t.Fatalf("decoded %#x = %T/%#v", bits, decoded, decoded)
		}
		if got := math.Float64bits(literal.Value().(float64)); got != bits {
			t.Fatalf("decoded bits=%#x, want %#x", got, bits)
		}
		if !TypeEquals(original, decoded) {
			t.Fatalf("decoded %#x lost type identity", bits)
		}
		roundTrip, err := EncodeCanonical(context.Background(), decoded)
		if err != nil || !bytes.Equal(encoded, roundTrip) {
			t.Fatalf("round trip %#x changed bytes: %x / %x / %v", bits, encoded, roundTrip, err)
		}
	}
}

func TestDecodeCanonicalRejectsRecursiveAuthorityAndActiveBackrefs(t *testing.T) {
	for name, value := range map[string]Type{
		"acyclic-recursive-node": NewRecursive("Named", func(Type) Type { return String }),
		"self-cycle":             NewRecursive("Named", func(self Type) Type { return NewArray(self) }),
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := EncodeCanonical(context.Background(), value)
			if err != nil {
				t.Fatal(err)
			}
			if decoded, err := DecodeCanonical(context.Background(), encoded); !errors.Is(err, ErrCanonicalRecursiveIdentityUnavailable) || decoded != nil {
				t.Fatalf("DecodeCanonical = %T, %v", decoded, err)
			}
		})
	}

	// A forged structural cycle without an explicit Recursive node is still an
	// active back-reference and cannot be reconstructed as portable authority.
	encoded := appendFrameString(nil, canonicalTypeDomain)
	encoded = binary.AppendUvarint(encoded, canonicalTypeVersion)
	encoded = append(encoded, 1)               // definition
	encoded = binary.AppendUvarint(encoded, 0) // ordinal zero
	encoded = appendFrameBytes(encoded, []byte{canonicalArray})
	encoded = binary.AppendUvarint(encoded, 1) // one child
	encoded = append(encoded, 0)               // reference
	encoded = binary.AppendUvarint(encoded, 0) // active ordinal zero
	if decoded, err := DecodeCanonical(context.Background(), encoded); !errors.Is(err, ErrCanonicalRecursiveIdentityUnavailable) || decoded != nil {
		t.Fatalf("active backref = %T, %v", decoded, err)
	}
}

func TestDecodeCanonicalStructuralRoundTripsRecursiveGraph(t *testing.T) {
	left := NewRecursivePlaceholder("Left")
	right := NewRecursivePlaceholder("Right")
	left.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "right", Type: right}}}))
	right.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "left", Type: left}, {Name: "value", Type: String}}}))

	encoded, err := EncodeCanonical(context.Background(), left)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil {
		t.Fatalf("DecodeCanonicalStructural: %v", err)
	}
	if !TypeEquals(left, decoded) {
		t.Fatalf("decoded recursive graph = %v, want structural equality with %v", decoded, left)
	}
	roundTrip, err := EncodeCanonical(context.Background(), decoded)
	if err != nil || !bytes.Equal(encoded, roundTrip) {
		t.Fatalf("structural recursive bytes changed: %x / %x / %v", encoded, roundTrip, err)
	}

	// An active edge without a Recursive binder cannot manufacture a type
	// through the structural decoder.
	forged := appendFrameString(nil, canonicalTypeDomain)
	forged = binary.AppendUvarint(forged, canonicalTypeVersion)
	forged = append(forged, 1) // definition
	forged = binary.AppendUvarint(forged, 0)
	forged = appendFrameBytes(forged, []byte{canonicalArray})
	forged = binary.AppendUvarint(forged, 1)
	forged = append(forged, 0) // active reference
	forged = binary.AppendUvarint(forged, 0)
	if decoded, err := DecodeCanonicalStructural(context.Background(), forged); !errors.Is(err, ErrCanonicalRecursiveIdentityUnavailable) || decoded != nil {
		t.Fatalf("unbound structural backref = %T, %v", decoded, err)
	}
}

func TestDecodeCanonicalStructuralRoundTripsRecursiveGenericGraph(t *testing.T) {
	param := NewTypeParam("T", nil)
	collection := NewGeneric("Collection", []*TypeParam{param}, nil)
	collection.SetBody(RebuildRecord(RecordParts{
		Fields: []Field{{Name: "next", Type: Instantiate(collection, param)}},
	}))

	encoded, err := EncodeCanonical(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil {
		t.Fatalf("DecodeCanonicalStructural: %v", err)
	}
	if !TypeEquals(collection, decoded) {
		t.Fatalf("decoded generic graph = %v, want structural equality with %v", decoded, collection)
	}
	roundTrip, err := EncodeCanonical(context.Background(), decoded)
	if err != nil || !bytes.Equal(encoded, roundTrip) {
		t.Fatalf("structural generic bytes changed: %x / %x / %v", encoded, roundTrip, err)
	}
}

func TestDecodeCanonicalRejectsMalformedTrailingAndImpossibleCounts(t *testing.T) {
	valid := mustCanonical(t, NewTuple(String, Number))
	trailing := append(append([]byte(nil), valid...), 0)
	if decoded, err := DecodeCanonical(context.Background(), trailing); !errors.Is(err, ErrInvalidCanonicalType) || decoded != nil {
		t.Fatalf("trailing = %T, %v", decoded, err)
	}

	forward := appendFrameString(nil, canonicalTypeDomain)
	forward = binary.AppendUvarint(forward, canonicalTypeVersion)
	forward = append(forward, 0)
	forward = binary.AppendUvarint(forward, 1)
	if decoded, err := DecodeCanonical(context.Background(), forward); !errors.Is(err, ErrInvalidCanonicalType) || decoded != nil {
		t.Fatalf("forward root = %T, %v", decoded, err)
	}

	huge := appendFrameString(nil, canonicalTypeDomain)
	huge = binary.AppendUvarint(huge, canonicalTypeVersion)
	huge = append(huge, 1)
	huge = binary.AppendUvarint(huge, 0)
	huge = appendFrameBytes(huge, []byte{canonicalTuple, 1})
	huge = binary.AppendUvarint(huge, ^uint64(0))
	if decoded, err := DecodeCanonical(context.Background(), huge); !errors.Is(err, ErrInvalidCanonicalType) || decoded != nil {
		t.Fatalf("impossible count = %T, %v", decoded, err)
	}
}

func TestDecodeCanonicalCancellationPublishesNothing(t *testing.T) {
	children := make([]Type, 4096)
	for index := range children {
		children[index] = NewTuple(LiteralInt(int64(index)), NewArray(String))
	}
	encoded := mustCanonical(t, NewTuple(children...))
	ctx := &canonicalCancelContext{remaining: 3}
	if decoded, err := DecodeCanonical(ctx, encoded); !errors.Is(err, context.Canceled) || decoded != nil {
		t.Fatalf("DecodeCanonical = %T, %v", decoded, err)
	}
}

// TestDecodeCanonicalStructuralRoundTripsRecursiveUnionMember covers a union
// whose member is a recursive binder. Member order is fixed by hash when the
// union node is built, and a structural decode has only open placeholders at
// that moment, so re-sorting would publish a different order than the bytes
// carry and reject the encoder's own output.
func TestDecodeCanonicalStructuralRoundTripsRecursiveUnionMember(t *testing.T) {
	box := NewRecursivePlaceholder("Box")
	tombstone := RebuildRecord(RecordParts{Fields: []Field{{Name: "kind", Type: LiteralString("tombstone")}}})
	box.SetBody(RebuildRecord(RecordParts{Fields: []Field{
		{Name: "kind", Type: LiteralString("box")},
		{Name: "next", Type: MaterializeUnion([]Type{box, tombstone})},
	}}))
	payload := MaterializeUnion([]Type{tombstone, box})

	encoded, err := EncodeCanonical(context.Background(), payload)
	if err != nil {
		t.Fatalf("EncodeCanonical: %v", err)
	}
	decoded, err := DecodeCanonicalStructural(context.Background(), encoded)
	if err != nil {
		t.Fatalf("DecodeCanonicalStructural rejected its own encoding: %v", err)
	}
	if !TypeEquals(decoded, payload) {
		t.Fatalf("structural decode changed the recursive union: %s", decoded)
	}
	again, err := EncodeCanonical(context.Background(), decoded)
	if err != nil || !bytes.Equal(again, encoded) {
		t.Fatalf("re-encoding the decoded recursive union is not byte-stable (err=%v)", err)
	}
}

// TestDecodeCanonicalStructuralRejectsReorderedUnionMembers keeps the
// canonical order requirement for graphs without a recursive binder: those
// member hashes are already final, so a reordered stream is not canonical.
func TestDecodeCanonicalStructuralRejectsReorderedUnionMembers(t *testing.T) {
	canonical, err := EncodeCanonical(context.Background(), MaterializeUnion([]Type{String, Integer, Boolean}))
	if err != nil {
		t.Fatalf("EncodeCanonical: %v", err)
	}
	reordered, err := EncodeCanonical(context.Background(), &Union{Members: reversedTypes(MaterializeUnion([]Type{String, Integer, Boolean}).(*Union).Members)})
	if err != nil {
		t.Fatalf("EncodeCanonical reordered: %v", err)
	}
	if bytes.Equal(canonical, reordered) {
		t.Fatal("reordered union produced canonical bytes; the test cannot observe the rejection")
	}
	if decoded, err := DecodeCanonicalStructural(context.Background(), reordered); err == nil || decoded != nil {
		t.Fatalf("reordered union member stream was admitted: %v", decoded)
	}
}

func reversedTypes(members []Type) []Type {
	out := make([]Type, len(members))
	for index, member := range members {
		out[len(members)-1-index] = member
	}
	return out
}
