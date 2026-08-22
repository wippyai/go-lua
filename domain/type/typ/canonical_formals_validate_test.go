package typ

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestAdmitCanonicalFormalsAcceptsEncoderCorpus(t *testing.T) {
	external := NewTypeParam("\x00external\xff", LiteralString("constraint\x00\xff"))
	local := NewTypeParam("local", nil)
	function := Func().TypeParamRef(local).Param("value", local).Returns(external, local).Build()
	channelParam := NewTypeParam("item", nil)
	channel := NewGeneric("Channel\x00\xff", []*TypeParam{channelParam}, NewArray(channelParam))
	record := RebuildRecord(RecordParts{
		Fields: []Field{{Name: "field\x00\xff", Type: external, Optional: true, Readonly: true}},
		StaticMembers: []StaticMember{
			{Kind: StaticMemberStringIndex, Name: "member\x00\xff", Type: LiteralString("value\x00\xff"), Readonly: true},
			{Kind: StaticMemberIntIndex, Index: -7, Type: String},
		},
		MapKey: String, MapValue: channel, Metatable: NewMeta(function), Open: true, AssumeSorted: true,
	})
	recursive := NewRecursive("presentation-only", func(self Type) Type {
		return NewTuple(self, external)
	})
	recursiveParam := NewTypeParam("T", nil)
	recursiveGeneric := NewGeneric("Recursive", []*TypeParam{recursiveParam}, nil)
	recursiveGeneric.SetBody(RebuildRecord(RecordParts{
		Fields: []Field{{Name: "next", Type: Instantiate(recursiveGeneric, recursiveParam)}},
	}))
	allScalars := NewTuple(
		Nil, Boolean, Number, Integer, String, Any, Unknown, Never, Self,
		LiteralBool(true), LiteralInt(-19), LiteralNumber(2.5), LiteralString("literal\x00\xff"),
		NewRef("module\x00\xff", "name\x00\xff"),
		MaterializeOptional(String),
		MaterializeUnion([]Type{String, Integer}),
		MaterializeIntersection([]Type{String, Integer}),
		NewArray(String), NewMap(String, Integer), NewReadonlyMap(String, Integer),
	)
	noBodyParam := NewTypeParam("T", nil)
	noBodyGeneric := NewGeneric("NoBody", []*TypeParam{noBodyParam}, nil)

	corpus := []struct {
		name    string
		value   Type
		formals []*TypeParam
		formalN int
	}{
		{"nil", nil, nil, 0},
		{"all-scalar-tags", allScalars, nil, 0},
		{"external", NewTuple(external, NewArray(external)), []*TypeParam{external}, 1},
		{"function", function, []*TypeParam{external}, 1},
		{"generic", channel, nil, 0},
		{"instantiated", Instantiate(channel, String), nil, 0},
		{"record", record, []*TypeParam{external}, 1},
		{"interface", NewInterface("Reader\x00\xff", []Method{{Name: "read\x00\xff", Type: function}}), []*TypeParam{external}, 1},
		{"recursive", recursive, []*TypeParam{external}, 1},
		{"recursive-without-body", NewRecursivePlaceholder("unbound"), nil, 0},
		{"recursive-generic", recursiveGeneric, nil, 0},
		{"generic-without-body", noBodyGeneric, nil, 0},
		{"nested-binders", canonicalScopedNested("outer", "inner"), nil, 0},
	}
	for _, test := range corpus {
		t.Run(test.name, func(t *testing.T) {
			receipt, err := EncodeCanonicalFormals(context.Background(), test.value, test.formals)
			if err != nil {
				t.Fatalf("EncodeCanonicalFormals: %v", err)
			}
			admitted, err := AdmitCanonicalFormals(context.Background(), receipt.Bytes(), test.formalN)
			if err != nil {
				t.Fatalf("AdmitCanonicalFormals rejected encoder bytes: %v\n%x", err, receipt.Bytes())
			}
			if !admitted.Valid() {
				t.Fatal("AdmitCanonicalFormals returned an invalid receipt")
			}
		})
	}
}

func TestAdmitCanonicalFormalsScopeLaws(t *testing.T) {
	leftFormal := NewTypeParam("T", LiteralString("\x00\xff"))
	rightFormal := NewTypeParam("Renamed", LiteralString("\x00\xff"))
	left, err := EncodeCanonicalFormals(context.Background(), NewTuple(leftFormal, NewArray(leftFormal)), []*TypeParam{leftFormal})
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeCanonicalFormals(context.Background(), NewTuple(rightFormal, NewArray(rightFormal)), []*TypeParam{rightFormal})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.Bytes(), right.Bytes()) {
		t.Fatalf("alpha-equivalent bytes differ:\n%x\n%x", left.Bytes(), right.Bytes())
	}
	if _, err := AdmitCanonicalFormals(context.Background(), left.Bytes(), 1); err != nil {
		t.Fatalf("alpha-invariant bytes rejected: %v", err)
	}
	if _, err := AdmitCanonicalFormals(context.Background(), left.Bytes(), 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("external formal outside receiver scope = %v", err)
	}
	if _, err := AdmitCanonicalFormals(context.Background(), left.Bytes(), -1); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("negative receiver scope = %v", err)
	}

	first := NewTypeParam("First", nil)
	second := NewTypeParam("Second", nil)
	forward, err := EncodeCanonicalFormals(context.Background(), NewTuple(first, second), []*TypeParam{first, second})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := EncodeCanonicalFormals(context.Background(), NewTuple(first, second), []*TypeParam{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(forward.Bytes(), reversed.Bytes()) {
		t.Fatal("formal order lost its semantic ordinal")
	}
	for _, receipt := range []CanonicalFormalsReceipt{forward, reversed} {
		if _, err := AdmitCanonicalFormals(context.Background(), receipt.Bytes(), 2); err != nil {
			t.Fatalf("formal-order encoding rejected: %v", err)
		}
	}
}

func TestAdmitCanonicalFormalsRejectsMalformedAndNoncanonicalBytes(t *testing.T) {
	valid, err := EncodeCanonicalFormals(context.Background(), NewTuple(String, Number), nil)
	if err != nil {
		t.Fatal(err)
	}
	nonMinimalVersion := appendCanonicalFormalsDomain(nil)
	nonMinimalVersion = append(nonMinimalVersion, 0x81, 0x00)
	nonMinimalVersion = appendCanonicalFormalDefinition(nonMinimalVersion, 0, []byte{canonicalString}, 0)
	nonMinimalGraphOrdinal := appendCanonicalFormalsVersion(nil)
	nonMinimalGraphOrdinal = append(nonMinimalGraphOrdinal, 1, 0x80, 0x00)
	nonMinimalGraphOrdinal = appendFrameBytes(nonMinimalGraphOrdinal, []byte{canonicalString})
	nonMinimalGraphOrdinal = binary.AppendUvarint(nonMinimalGraphOrdinal, 0)
	nonMinimalScalarCount := appendCanonicalFormalsVersion(nil)
	nonMinimalScalarCount = appendCanonicalFormalDefinition(nonMinimalScalarCount, 0, []byte{canonicalTuple, 0x80, 0x00}, 0)
	nonMinimalFormalOrdinal := appendCanonicalFormalsVersion(nil)
	nonMinimalFormalOrdinal = appendCanonicalFormalDefinition(nonMinimalFormalOrdinal, 0, []byte{canonicalTypeParam, canonicalScopedExternalFormal, 0x80, 0x00, 0}, 0)

	badScalarArity := appendCanonicalFormalsDomain(nil)
	badScalarArity = binary.AppendUvarint(badScalarArity, canonicalScopedTypeVersion)
	badScalarArity = appendCanonicalFormalDefinition(badScalarArity, 0, []byte{canonicalOptional}, 0)

	badBoolean := appendCanonicalFormalsDomain(nil)
	badBoolean = binary.AppendUvarint(badBoolean, canonicalScopedTypeVersion)
	badBoolean = appendCanonicalFormalDefinition(badBoolean, 0, []byte{canonicalLiteral, byte(Boolean.Kind()), 2}, 0)

	badExternal := appendCanonicalFormalsDomain(nil)
	badExternal = binary.AppendUvarint(badExternal, canonicalScopedTypeVersion)
	badExternal = appendCanonicalFormalDefinition(badExternal, 0, []byte{canonicalTypeParam, canonicalScopedExternalFormal, 1, 0}, 0)

	// This local formal uses an active self edge where a Function or Generic
	// lexical owner is required. It is a structurally well-framed cyclic graph,
	// but not a valid scoped binder graph.
	badLocalCycle := appendCanonicalFormalsDomain(nil)
	badLocalCycle = binary.AppendUvarint(badLocalCycle, canonicalScopedTypeVersion)
	badLocalCycle = appendCanonicalFormalDefinition(badLocalCycle, 0, []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 0}, 1)
	badLocalCycle = appendCanonicalFormalReference(badLocalCycle, 0)

	badInstantiated := appendCanonicalFormalsDomain(nil)
	badInstantiated = binary.AppendUvarint(badInstantiated, canonicalScopedTypeVersion)
	badInstantiated = appendCanonicalFormalDefinition(badInstantiated, 0, []byte{canonicalInstantiated, 0}, 1)
	badInstantiated = appendCanonicalFormalDefinition(badInstantiated, 1, []byte{canonicalString}, 0)

	badInterface := appendCanonicalFormalsDomain(nil)
	badInterface = binary.AppendUvarint(badInterface, canonicalScopedTypeVersion)
	badInterfaceScalar := appendFrameString([]byte{canonicalInterface}, "I")
	badInterfaceScalar = binary.AppendUvarint(badInterfaceScalar, 1)
	badInterfaceScalar = appendFrameString(badInterfaceScalar, "read")
	badInterface = appendCanonicalFormalDefinition(badInterface, 0, badInterfaceScalar, 1)
	badInterface = appendCanonicalFormalDefinition(badInterface, 1, []byte{canonicalString}, 0)

	// Two identical String definitions are grammatically complete but are not
	// the quotient emitted by canonicalEncoder; the second use must be a ref.
	nonCanonicalDuplicate := appendCanonicalFormalsDomain(nil)
	nonCanonicalDuplicate = binary.AppendUvarint(nonCanonicalDuplicate, canonicalScopedTypeVersion)
	nonCanonicalDuplicate = appendCanonicalFormalDefinition(nonCanonicalDuplicate, 0, []byte{canonicalTuple, 2}, 2)
	nonCanonicalDuplicate = appendCanonicalFormalDefinition(nonCanonicalDuplicate, 1, []byte{canonicalString}, 0)
	nonCanonicalDuplicate = appendCanonicalFormalDefinition(nonCanonicalDuplicate, 2, []byte{canonicalString}, 0)

	tests := map[string][]byte{
		"wrong domain":               appendCanonicalFormalDefinition(nil, 0, []byte{canonicalString}, 0),
		"non-minimal version":        nonMinimalVersion,
		"non-minimal graph ordinal":  nonMinimalGraphOrdinal,
		"non-minimal scalar count":   nonMinimalScalarCount,
		"non-minimal formal ordinal": nonMinimalFormalOrdinal,
		"trailing byte":              append(append([]byte(nil), valid.Bytes()...), 0),
		"unknown graph opcode":       append(appendCanonicalFormalsVersion(nil), 2, 0),
		"forward root reference":     appendCanonicalFormalReference(appendCanonicalFormalsVersion(nil), 0),
		"bad scalar arity":           badScalarArity,
		"bad scalar boolean":         badBoolean,
		"external ordinal":           badExternal,
		"malformed local cycle":      badLocalCycle,
		"instantiated child":         badInstantiated,
		"interface child":            badInterface,
		"duplicate graph node":       nonCanonicalDuplicate,
		"truncated definition":       appendCanonicalFormalsVersion(nil),
		"non-dense definition":       appendCanonicalFormalDefinition(appendCanonicalFormalsVersion(nil), 1, []byte{canonicalString}, 0),
		"unknown scalar":             appendCanonicalFormalDefinition(appendCanonicalFormalsVersion(nil), 0, []byte{0xff}, 0),
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := AdmitCanonicalFormals(context.Background(), encoded, 1); !errors.Is(err, ErrInvalidCanonicalType) {
				t.Fatalf("AdmitCanonicalFormals(%x) = %v, want invalid canonical error", encoded, err)
			}
		})
	}
	validBytes := valid.Bytes()
	for index := 0; index < len(validBytes); index++ {
		if _, err := AdmitCanonicalFormals(context.Background(), validBytes[:index], 0); !errors.Is(err, ErrInvalidCanonicalType) {
			t.Fatalf("truncated valid[:%d] = %v", index, err)
		}
	}
}

func TestAdmitCanonicalFormalsDeepGraphUsesNoGoStack(t *testing.T) {
	const depth = 100_001
	var value Type = String
	for range depth {
		value = &Optional{Inner: value}
	}
	receipt, err := EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	if _, err := AdmitCanonicalFormals(context.Background(), receipt.Bytes(), 0); err != nil {
		t.Fatalf("AdmitCanonicalFormals deep graph: %v", err)
	}
}

func FuzzAdmitCanonicalFormals(f *testing.F) {
	seed := appendCanonicalFormalsVersion(nil)
	seed = appendCanonicalFormalDefinition(seed, 0, []byte{canonicalString}, 0)
	f.Add(seed, uint8(0))
	f.Add([]byte{}, uint8(0))
	f.Add([]byte{0xff, 0x80, 0x00}, uint8(3))

	f.Fuzz(func(t *testing.T, encoded []byte, externalCount uint8) {
		_, _ = AdmitCanonicalFormals(context.Background(), encoded, int(externalCount))
	})
}

func appendCanonicalFormalsDomain(out []byte) []byte {
	return appendFrameString(out, canonicalScopedTypeDomain)
}

func appendCanonicalFormalsVersion(out []byte) []byte {
	out = appendCanonicalFormalsDomain(out)
	return binary.AppendUvarint(out, canonicalScopedTypeVersion)
}

func appendCanonicalFormalDefinition(out []byte, ordinal uint64, scalar []byte, children uint64) []byte {
	out = append(out, 1)
	out = binary.AppendUvarint(out, ordinal)
	out = appendFrameBytes(out, scalar)
	return binary.AppendUvarint(out, children)
}

func appendCanonicalFormalReference(out []byte, ordinal uint64) []byte {
	out = append(out, 0)
	return binary.AppendUvarint(out, ordinal)
}
