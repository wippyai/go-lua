package wire

import (
	"strings"
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func TestWireRejectsMalformedTypeWireMissingRequiredParts(t *testing.T) {
	tests := []struct {
		name string
		wire *TypeWire
		want string
	}{
		{
			name: "map missing key",
			wire: &TypeWire{Kind: "map", Value: &TypeWire{Kind: "number"}},
			want: "map key missing type",
		},
		{
			name: "readonly map missing value",
			wire: &TypeWire{Kind: "readonlyMap", Key: &TypeWire{Kind: "string"}},
			want: "readonly map value missing type",
		},
		{
			name: "record map missing value",
			wire: &TypeWire{Kind: "record", MapKey: &TypeWire{Kind: "string"}},
			want: "record map value missing type",
		},
		{
			name: "annotation missing name",
			wire: &TypeWire{
				Kind:        "annotated",
				Element:     &TypeWire{Kind: "string"},
				Annotations: []annotationWire{{Kind: "nil"}},
			},
			want: "annotation missing name",
		},
		{
			name: "annotation missing kind",
			wire: &TypeWire{
				Kind:        "annotated",
				Element:     &TypeWire{Kind: "string"},
				Annotations: []annotationWire{{Name: "tag"}},
			},
			want: `annotation "tag" missing arg kind`,
		},
		{
			name: "function parameter missing type",
			wire: &TypeWire{
				Kind:   "function",
				Params: []paramWire{{Name: "value"}},
			},
			want: "function parameter 0 missing type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeType(tt.wire)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeType error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWireRecordStaticIntMemberRequiresExplicitIndex(t *testing.T) {
	stringType, err := EncodeType(typ.String)
	if err != nil {
		t.Fatalf("EncodeType: %v", err)
	}
	_, err = DecodeType(&TypeWire{
		Kind: "record",
		StaticMembers: []staticMemberWire{{
			Kind: "int",
			Type: stringType,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "static member int index missing") {
		t.Fatalf("DecodeType error = %v, want missing static member int index", err)
	}
}

func TestWireRecordStaticIntMemberEncodesZeroIndexExplicitly(t *testing.T) {
	wire, err := EncodeType(typetable.NewRecord().StaticIntIndex(0, typ.String).Build())
	if err != nil {
		t.Fatalf("EncodeType: %v", err)
	}
	if wire == nil || len(wire.StaticMembers) != 1 || wire.StaticMembers[0].Index == nil || *wire.StaticMembers[0].Index != 0 {
		t.Fatalf("record wire = %#v, want explicit static int index 0", wire)
	}
}

func TestWireRoundTripPreservesSharedRecursiveFamily(t *testing.T) {
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

	wire, err := EncodeType(root)
	if err != nil {
		t.Fatalf("EncodeType: %v", err)
	}
	decoded, err := DecodeType(wire)
	if err != nil {
		t.Fatalf("DecodeType: %v", err)
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

func TestDecodeRejectsOutOfScopeRecursiveReference(t *testing.T) {
	_, err := DecodeType(&TypeWire{Kind: "recursiveRef", Binder: 1})
	if err == nil {
		t.Fatal("Decode accepted an out-of-scope recursive reference")
	}
}

func TestDecodeRejectsDuplicateRecursiveBinder(t *testing.T) {
	wire := &TypeWire{
		Kind: "record",
		Fields: []fieldWire{
			{Name: "first", Type: &TypeWire{Kind: "recursive", Binder: 1, Name: "First", Body: &TypeWire{Kind: "string"}}},
			{Name: "second", Type: &TypeWire{Kind: "recursive", Binder: 1, Name: "Second", Body: &TypeWire{Kind: "number"}}},
		},
	}
	_, err := DecodeType(wire)
	if err == nil {
		t.Fatal("Decode accepted a duplicate recursive binder")
	}
}
