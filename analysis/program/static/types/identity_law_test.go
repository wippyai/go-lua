package types

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyTypePrimitive] = 5
	counts[keyspace.FamilyTypeLiteral] = 2
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	counts[keyspace.FamilyTypeIntersection] = 1
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypeGeneric] = 1
	counts[keyspace.FamilyTypeArray] = 1
	counts[keyspace.FamilyTypeMap] = 1
	counts[keyspace.FamilyTypeRecord] = 1
	counts[keyspace.FamilyTypeField] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func primitiveTerm(ordinal uint32) keyspace.Term {
	return term(keyspace.FamilyTypePrimitive, ordinal)
}

// ledgerInput is one complete authored forest exercising every relation and
// every variable-width column. Each ledger case perturbs exactly one authored
// distinction of this input and must remain admissible.
func ledgerInput() Input {
	return Input{
		Primitive: []Primitive{
			{Kind: PrimitiveNil}, {Kind: PrimitiveNumber}, {Kind: PrimitiveString},
			{Kind: PrimitiveBoolean}, {Kind: PrimitiveNever},
		},
		Literal: []Literal{
			{Kind: keyspace.LiteralString, Exact: 7},
			{Kind: keyspace.LiteralFloat},
		},
		Optional:     []Optional{{Inner: primitiveTerm(1)}},
		Union:        []Union{{Members: []keyspace.Term{term(keyspace.FamilyTypeOptional, 1), primitiveTerm(2)}}},
		Intersection: []Intersection{{Members: []keyspace.Term{primitiveTerm(3), primitiveTerm(4)}}},
		Generic: []Generic{{
			Base: term(keyspace.FamilyTypeRef, 1),
			Args: []keyspace.Term{term(keyspace.FamilyTypeUnion, 1)},
		}},
		Array: []Array{{Element: term(keyspace.FamilyTypeGeneric, 1), ReadOnly: true}},
		Map:   []Map{{Key: term(keyspace.FamilyTypeRef, 2), Value: term(keyspace.FamilyTypeArray, 1)}},
		Field: []Field{
			{Key: 9, Type: term(keyspace.FamilyTypeMap, 1), Optional: true},
			{Key: 10, Type: primitiveTerm(5)},
		},
		Record: []Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 1)}, ReadOnly: true}},
	}
}

func sectionReader(t *testing.T, data []byte) *framing.Reader {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

const (
	sectionDomain  = "program/static/types-law"
	sectionVersion = 1
)

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field, arity, and order distinction this vertical retains. A distinction the
// stream loses would let two different authored forests share one identity.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*Input)
	}{
		{"primitive.kind", func(in *Input) { in.Primitive[0].Kind = PrimitiveString }},
		{"primitive.order", func(in *Input) { in.Primitive[0], in.Primitive[1] = in.Primitive[1], in.Primitive[0] }},
		{"literal.kind", func(in *Input) { in.Literal[0].Kind = keyspace.LiteralInteger }},
		{"literal.exact", func(in *Input) { in.Literal[0].Exact = 77 }},
		{"literal.float-bits", func(in *Input) { in.Literal[1].FloatBits = 3 }},
		{"literal.order", func(in *Input) { in.Literal[0], in.Literal[1] = in.Literal[1], in.Literal[0] }},
		{"optional.inner", func(in *Input) { in.Optional[0].Inner = primitiveTerm(2) }},
		{"union.member", func(in *Input) { in.Union[0].Members[1] = primitiveTerm(3) }},
		{"union.arity", func(in *Input) { in.Union[0].Members = append(in.Union[0].Members, primitiveTerm(5)) }},
		{"union.order", func(in *Input) {
			in.Union[0].Members[0], in.Union[0].Members[1] = in.Union[0].Members[1], in.Union[0].Members[0]
		}},
		{"intersection.member", func(in *Input) { in.Intersection[0].Members[1] = primitiveTerm(5) }},
		{"intersection.arity", func(in *Input) {
			in.Intersection[0].Members = append(in.Intersection[0].Members, primitiveTerm(5))
		}},
		{"intersection.order", func(in *Input) {
			in.Intersection[0].Members[0], in.Intersection[0].Members[1] = in.Intersection[0].Members[1], in.Intersection[0].Members[0]
		}},
		{"generic.base", func(in *Input) { in.Generic[0].Base = term(keyspace.FamilyTypeRef, 2) }},
		{"generic.arg", func(in *Input) { in.Generic[0].Args[0] = term(keyspace.FamilyTypeIntersection, 1) }},
		{"generic.arity", func(in *Input) { in.Generic[0].Args = append(in.Generic[0].Args, primitiveTerm(1)) }},
		{"array.element", func(in *Input) { in.Array[0].Element = term(keyspace.FamilyTypeOptional, 1) }},
		{"array.read-only", func(in *Input) { in.Array[0].ReadOnly = false }},
		{"map.key", func(in *Input) { in.Map[0].Key = term(keyspace.FamilyTypeRef, 1) }},
		{"map.value", func(in *Input) { in.Map[0].Value = term(keyspace.FamilyTypeOptional, 1) }},
		{"map.read-only", func(in *Input) { in.Map[0].ReadOnly = true }},
		{"record.field", func(in *Input) { in.Record[0].Fields[0] = term(keyspace.FamilyTypeField, 2) }},
		{"record.arity", func(in *Input) { in.Record[0].Fields = nil }},
		{"record.read-only", func(in *Input) { in.Record[0].ReadOnly = false }},
		{"field.key", func(in *Input) { in.Field[0].Key = 88 }},
		{"field.type", func(in *Input) { in.Field[0].Type = term(keyspace.FamilyTypeOptional, 1) }},
		{"field.optional", func(in *Input) { in.Field[0].Optional = false }},
		{"field.order", func(in *Input) { in.Field[0], in.Field[1] = in.Field[1], in.Field[0] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput())
			perturbed := ledgerInput()
			test.perturb(&perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted, so a rebuild from
// the artifact reproduces the same sealed forest and the same identity.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	original := ledgerInput()
	encoded := sectionBytes(t, original)

	reader := sectionReader(t, encoded)
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(encoded, sectionBytes(t, decoded)) {
		t.Fatal("round-tripped input did not reproduce the section stream")
	}
}

// TestScanValidatesWithoutRetainingRows proves the preflight half consumes the
// same stream shape as Decode, which is what lets the enclosing owner reject a
// hostile payload before allocating any row.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}
