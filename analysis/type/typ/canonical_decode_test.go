package typ

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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
