package declarations

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/declarations-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyCell] = 3
	counts[keyspace.FamilyTypeAlias] = 2
	counts[keyspace.FamilyTypeParam] = 2
	counts[keyspace.FamilyTypeInterface] = 2
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypeField] = 2
	counts[keyspace.FamilyTypeFunction] = 2
	counts[keyspace.FamilyTypePrimitive] = 2
	counts[keyspace.FamilyDeclaredType] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func coordinate(t *testing.T, startLine, startColumn, endLine, endColumn uint32) source.Coordinate {
	t.Helper()
	value, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	if !ok {
		t.Fatal("CoordinateFromParts rejected the ledger fixture")
	}
	return value
}

// ledgerInput is one complete authored declaration set exercising every
// relation, both interface member kinds, and every variable-width column.
func ledgerInput(t *testing.T) Input {
	t.Helper()
	first := coordinate(t, 1, 1, 1, 2)
	second := coordinate(t, 2, 3, 2, 7)
	return Input{
		Alias: []TypeAlias{
			{
				Owner: term(keyspace.FamilyBody, 1), Target: term(keyspace.FamilyTypePrimitive, 1),
				Name: 1, NameCoordinate: first,
				Params: []keyspace.Term{term(keyspace.FamilyTypeParam, 1)},
			},
			{
				Owner: term(keyspace.FamilyBody, 2), Target: term(keyspace.FamilyTypePrimitive, 2),
				Name: 2, NameCoordinate: second,
			},
		},
		TypeParam: []TypeParam{
			{Owner: term(keyspace.FamilyTypeAlias, 1), Name: 3, Constraint: term(keyspace.FamilyTypePrimitive, 1)},
			{Owner: term(keyspace.FamilyTypeAlias, 2), Name: 4},
		},
		Interface: []Interface{
			{
				Owner: term(keyspace.FamilyBody, 1), Name: 5, NameCoordinate: first,
				Extends: []keyspace.Term{term(keyspace.FamilyTypeRef, 1), term(keyspace.FamilyTypeRef, 2)},
				Members: []InterfaceMember{
					{Kind: InterfaceField, Field: term(keyspace.FamilyTypeField, 1)},
					{
						Kind: InterfaceMethod, Name: 6, NameCoordinate: second,
						Signature: term(keyspace.FamilyTypeFunction, 1),
					},
				},
			},
			{Owner: term(keyspace.FamilyBody, 2), Name: 7, NameCoordinate: second},
		},
		DeclaredType: []DeclaredType{
			{Cell: term(keyspace.FamilyCell, 1), Target: term(keyspace.FamilyTypePrimitive, 1)},
			{Cell: term(keyspace.FamilyCell, 2), Target: term(keyspace.FamilyTypePrimitive, 2)},
		},
	}
}

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

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field, arity, and order distinction this vertical retains. A distinction the
// stream loses would let two different authored declaration sets share one
// identity.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*testing.T, *Input)
	}{
		{"alias.owner", func(_ *testing.T, in *Input) { in.Alias[0].Owner = term(keyspace.FamilyBody, 2) }},
		{"alias.target", func(_ *testing.T, in *Input) {
			in.Alias[0].Target = term(keyspace.FamilyTypePrimitive, 2)
		}},
		{"alias.name", func(_ *testing.T, in *Input) { in.Alias[0].Name = 77 }},
		{"alias.coordinate", func(t *testing.T, in *Input) { in.Alias[0].NameCoordinate = coordinate(t, 4, 4, 4, 9) }},
		{"alias.param", func(_ *testing.T, in *Input) {
			in.Alias[0].Params[0] = term(keyspace.FamilyTypeParam, 2)
		}},
		{"alias.param-arity", func(_ *testing.T, in *Input) { in.Alias[0].Params = nil }},
		{"alias.order", func(_ *testing.T, in *Input) { in.Alias[0], in.Alias[1] = in.Alias[1], in.Alias[0] }},

		{"typeparam.owner", func(_ *testing.T, in *Input) {
			in.TypeParam[0].Owner = term(keyspace.FamilyTypeAlias, 2)
		}},
		{"typeparam.name", func(_ *testing.T, in *Input) { in.TypeParam[0].Name = 77 }},
		{"typeparam.constraint", func(_ *testing.T, in *Input) { in.TypeParam[0].Constraint = 0 }},
		{"typeparam.order", func(_ *testing.T, in *Input) {
			in.TypeParam[0], in.TypeParam[1] = in.TypeParam[1], in.TypeParam[0]
		}},

		{"interface.owner", func(_ *testing.T, in *Input) { in.Interface[0].Owner = term(keyspace.FamilyBody, 2) }},
		{"interface.name", func(_ *testing.T, in *Input) { in.Interface[0].Name = 77 }},
		{"interface.coordinate", func(t *testing.T, in *Input) {
			in.Interface[0].NameCoordinate = coordinate(t, 4, 4, 4, 9)
		}},
		{"interface.extends-member", func(_ *testing.T, in *Input) {
			in.Interface[0].Extends[0] = term(keyspace.FamilyTypeRef, 2)
		}},
		{"interface.extends-arity", func(_ *testing.T, in *Input) {
			in.Interface[0].Extends = in.Interface[0].Extends[:1]
		}},
		{"interface.extends-order", func(_ *testing.T, in *Input) {
			in.Interface[0].Extends[0], in.Interface[0].Extends[1] = in.Interface[0].Extends[1], in.Interface[0].Extends[0]
		}},
		{"interface.order", func(_ *testing.T, in *Input) {
			in.Interface[0], in.Interface[1] = in.Interface[1], in.Interface[0]
		}},

		// The member kind carries its exact-xor payload with it, so the
		// perturbation replaces the whole member rather than one field.
		{"member.kind", func(t *testing.T, in *Input) {
			in.Interface[0].Members[0] = InterfaceMember{
				Kind: InterfaceMethod, Name: 8, NameCoordinate: coordinate(t, 1, 1, 1, 2),
				Signature: term(keyspace.FamilyTypeFunction, 2),
			}
		}},
		{"member.field", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[0].Field = term(keyspace.FamilyTypeField, 2)
		}},
		{"member.name", func(_ *testing.T, in *Input) { in.Interface[0].Members[1].Name = 77 }},
		{"member.coordinate", func(t *testing.T, in *Input) {
			in.Interface[0].Members[1].NameCoordinate = coordinate(t, 4, 4, 4, 9)
		}},
		{"member.signature", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[1].Signature = term(keyspace.FamilyTypeFunction, 2)
		}},
		{"member.arity", func(_ *testing.T, in *Input) {
			in.Interface[0].Members = in.Interface[0].Members[:1]
		}},
		{"member.order", func(_ *testing.T, in *Input) {
			in.Interface[0].Members[0], in.Interface[0].Members[1] = in.Interface[0].Members[1], in.Interface[0].Members[0]
		}},

		{"declared-type.cell", func(_ *testing.T, in *Input) {
			in.DeclaredType[0].Cell = term(keyspace.FamilyCell, 3)
		}},
		{"declared-type.target", func(_ *testing.T, in *Input) {
			in.DeclaredType[0].Target = term(keyspace.FamilyTypePrimitive, 2)
		}},
		{"declared-type.order", func(_ *testing.T, in *Input) {
			in.DeclaredType[0], in.DeclaredType[1] = in.DeclaredType[1], in.DeclaredType[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput(t))
			perturbed := ledgerInput(t)
			test.perturb(t, &perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput(t))
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
// same stream shape as Decode.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput(t))
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}

// TestDeclaredTypeCellInverseTracksTheAuthoredRelation proves the dense Cell
// inverse is exactly the authored relation and nothing more: a Cell with no
// authored declared type resolves to nothing rather than to a stale ordinal.
func TestDeclaredTypeCellInverseTracksTheAuthoredRelation(t *testing.T) {
	table, err := Build(ledgerInput(t), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	view := table.View().DeclaredTypes()
	for index, cell := range []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)} {
		declared, ok := view.ForCell(cell)
		if !ok || declared != term(keyspace.FamilyDeclaredType, uint32(index+1)) {
			t.Fatalf("ForCell(%d) = %d/%v, want declared type %d", cell, declared, ok, index+1)
		}
	}
	if declared, ok := view.ForCell(term(keyspace.FamilyCell, 3)); ok || declared != 0 {
		t.Fatalf("undeclared Cell resolved to %d/%v", declared, ok)
	}
	if declared, ok := view.ForCell(term(keyspace.FamilyTypePrimitive, 1)); ok || declared != 0 {
		t.Fatalf("foreign-family term resolved to %d/%v", declared, ok)
	}
}

// TestBuildRefusesADuplicateDeclaredCell proves one Cell carries at most one
// authored declared type, which is what makes the dense inverse total.
func TestBuildRefusesADuplicateDeclaredCell(t *testing.T) {
	input := ledgerInput(t)
	input.DeclaredType[1].Cell = input.DeclaredType[0].Cell
	if _, err := Build(input, ledgerCounts()); err == nil {
		t.Fatal("Build admitted two declared types for one Cell")
	}
}
