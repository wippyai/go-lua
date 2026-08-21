package wire

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// sealedTypeWire states the integrity witness a producer would have written on
// a hand-built payload. A structural defect is only reachable once the payload
// is the one that was written, so a test that means to reach one seals first
// and the two obligations are exercised separately.
func sealedTypeWire(t *testing.T, w *TypeWire) *TypeWire {
	t.Helper()
	if err := sealTypeWire(w); err != nil {
		t.Fatalf("seal type wire: %v", err)
	}
	return w
}

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
			_, err := DecodeType(sealedTypeWire(t, tt.wire))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeType error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestWireRejectsAbsentTypeNodeInsteadOfDecodingIt states that an absent node
// inside a payload is a decode failure, not a decoded type. Absence is the
// caller's to express by not asking for a node at all; once a node is asked
// for, the decoder either states what it carries or refuses. Standing in a
// Go nil for it loses distinctions silently downstream: a union sheds the
// member it could not read and narrows, and an optional over nothing states a
// value that is definitely nil.
func TestWireRejectsAbsentTypeNodeInsteadOfDecodingIt(t *testing.T) {
	t.Run("absent node", func(t *testing.T) {
		decoded, err := DecodeType(nil)
		if err == nil {
			t.Fatalf("DecodeType(nil) = %v, want a decode error", decoded)
		}
	})
	t.Run("absent array element", func(t *testing.T) {
		decoded, err := DecodeType(sealedTypeWire(t, &TypeWire{Kind: "array"}))
		if err == nil {
			t.Fatalf("DecodeType(array with no element) = %v, want a decode error", decoded)
		}
	})
	t.Run("absent optional inner", func(t *testing.T) {
		decoded, err := DecodeType(sealedTypeWire(t, &TypeWire{Kind: "optional"}))
		if err == nil {
			t.Fatalf("DecodeType(optional over no element) = %v, want a decode error", decoded)
		}
	})
	t.Run("absent union member", func(t *testing.T) {
		want := typeexpr.Union(typ.String, typ.Number)
		decoded, err := DecodeType(sealedTypeWire(t, &TypeWire{
			Kind:    "union",
			Members: []*TypeWire{{Kind: "string"}, nil, {Kind: "number"}},
		}))
		if err == nil {
			t.Fatalf("DecodeType(union with an absent member) = %v, want a decode error", decoded)
		}
		if decoded != nil && decoded.String() == want.String() {
			t.Fatalf("union with an absent member decoded to the narrower %s", decoded)
		}
	})
}

func TestWireRecordStaticIntMemberRequiresExplicitIndex(t *testing.T) {
	_, err := DecodeType(sealedTypeWire(t, &TypeWire{
		Kind: "record",
		StaticMembers: []staticMemberWire{{
			Kind: "int",
			Type: &TypeWire{Kind: "string"},
		}},
	}))
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
	_, err := DecodeType(sealedTypeWire(t, &TypeWire{Kind: "recursiveRef", Binder: 1}))
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
	_, err := DecodeType(sealedTypeWire(t, wire))
	if err == nil {
		t.Fatal("Decode accepted a duplicate recursive binder")
	}
}

// TestWireRefusesASealedDegenerateMemberList states the structural half of the
// boundary, separately from the integrity witness. A witness answers whether
// the payload is the one that was written; it says nothing about whether what
// was written was writable. A union of one member and a union of none are
// neither: typ materializes a one-member list to that member and an empty one
// to never, so no encoder emits either, and a payload carrying one is a member
// list that lost members before it was sealed.
func TestWireRefusesASealedDegenerateMemberList(t *testing.T) {
	for _, probe := range []struct {
		name string
		wire *TypeWire
	}{
		{name: "single-member union", wire: &TypeWire{Kind: "union", Members: []*TypeWire{{Kind: "string"}}}},
		{name: "empty union", wire: &TypeWire{Kind: "union", Members: []*TypeWire{}}},
		{name: "member-less union", wire: &TypeWire{Kind: "union"}},
		{name: "single-member intersection", wire: &TypeWire{Kind: "intersection", Members: []*TypeWire{{Kind: "string"}}}},
		{name: "empty intersection", wire: &TypeWire{Kind: "intersection", Members: []*TypeWire{}}},
		{name: "member-less intersection", wire: &TypeWire{Kind: "intersection"}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			decoded, err := DecodeType(sealedTypeWire(t, probe.wire))
			if err == nil {
				t.Fatalf("a sealed %s decoded to %s", probe.name, decoded)
			}
			if !errors.Is(err, errDegenerateMemberList) {
				t.Fatalf("a sealed %s was refused as %v, not as a member list that lost members", probe.name, err)
			}
		})
	}
}

// TestWireEncoderRefusesADegenerateMemberList holds the same law on the write
// side. A boundary whose encoder can write a payload its decoder refuses has
// two answers to what a type is, and the producer learns which one at the far
// end of the wire instead of at the point it stated the type.
func TestWireEncoderRefusesADegenerateMemberList(t *testing.T) {
	for _, probe := range []struct {
		name  string
		value typ.Type
	}{
		{name: "single-member union", value: &typ.Union{Members: []typ.Type{typ.String}}},
		{name: "empty union", value: &typ.Union{}},
		{name: "single-member intersection", value: &typ.Intersection{Members: []typ.Type{typ.String}}},
		{name: "empty intersection", value: &typ.Intersection{}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			encoded, err := EncodeType(probe.value)
			if err == nil {
				t.Fatalf("the encoder wrote a %s as %#v", probe.name, encoded)
			}
			if !errors.Is(err, errDegenerateMemberList) {
				t.Fatalf("the encoder refused a %s as %v, not as a member list that lost members", probe.name, err)
			}
		})
	}
}

// TestWireRefusesAPayloadRewrittenAfterItWasWritten states what the integrity
// witness is for beyond absence. A rewritten payload is not malformed - every
// case below is a document the decoder would happily read - it is a different
// type from the one the producer stated, and every judgment made on it is made
// on evidence that was never sent.
func TestWireRefusesAPayloadRewrittenAfterItWasWritten(t *testing.T) {
	for _, probe := range []struct {
		name    string
		value   typ.Type
		rewrite func(string) string
	}{
		{
			name:    "atom kind",
			value:   typ.Integer,
			rewrite: func(payload string) string { return strings.Replace(payload, `"integer"`, `"number"`, 1) },
		},
		{
			name:    "literal digit",
			value:   typ.LiteralInt(1),
			rewrite: func(payload string) string { return strings.Replace(payload, `"int":1`, `"int":0`, 1) },
		},
		{
			name:    "literal truth",
			value:   typ.LiteralBool(true),
			rewrite: func(payload string) string { return strings.Replace(payload, `"bool":true`, `"bool":false`, 1) },
		},
		{
			name:  "field optionality",
			value: typetable.NewRecord().Field("id", typ.String).Build(),
			rewrite: func(payload string) string {
				return strings.Replace(payload, `"name":"id"`, `"name":"id","optional":true`, 1)
			},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			encoded, err := EncodeType(probe.value)
			if err != nil {
				t.Fatalf("EncodeType: %v", err)
			}
			payload, err := json.Marshal(encoded)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			rewritten := probe.rewrite(string(payload))
			if rewritten == string(payload) {
				t.Fatalf("the rewrite reached nothing in %s", payload)
			}
			var arrived TypeWire
			if err := json.Unmarshal([]byte(rewritten), &arrived); err != nil {
				t.Fatalf("the rewritten payload is not a document a reader would receive: %v", err)
			}
			decoded, err := DecodeType(&arrived)
			if err == nil {
				t.Fatalf("a payload rewritten in its %s decoded to %s; the producer stated %s",
					probe.name, decoded, probe.value)
			}
		})
	}
}

// TestWireCarriesAStringLiteralAsBytes states what a string literal is. A Lua
// string is an arbitrary byte sequence, and JSON text is not: written as a JSON
// string, every byte that is not valid UTF-8 becomes U+FFFD and the payload
// states a literal type nobody encoded. utf8.charpattern is exactly such a
// string, so a boundary that carries string literals as text carries the
// standard library wrong.
func TestWireCarriesAStringLiteralAsBytes(t *testing.T) {
	for _, probe := range []struct {
		name  string
		value string
	}{
		{name: "ascii", value: "NotFound"},
		{name: "empty", value: ""},
		{name: "utf8 text", value: "ключ"},
		{name: "utf8.charpattern", value: "[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"},
		{name: "lone continuation byte", value: "\x80"},
	} {
		t.Run(probe.name, func(t *testing.T) {
			encoded, err := EncodeType(typ.LiteralString(probe.value))
			if err != nil {
				t.Fatalf("EncodeType: %v", err)
			}
			payload, err := json.Marshal(encoded)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			var arrived TypeWire
			if err := json.Unmarshal(payload, &arrived); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			decoded, err := DecodeType(&arrived)
			if err != nil {
				t.Fatalf("DecodeType: %v", err)
			}
			literal, ok := decoded.(*typ.Literal)
			if !ok {
				t.Fatalf("decoded %T, want *typ.Literal", decoded)
			}
			if literal.Value() != probe.value {
				t.Fatalf("the literal arrived as %q, stated as %q", literal.Value(), probe.value)
			}
		})
	}
}
