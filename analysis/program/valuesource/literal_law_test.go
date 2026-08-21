package valuesource

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestLiteralEnumeratesTheFiveLiteralFamilies(t *testing.T) {
	input := valueSourceLawProgram(t)
	families := []keyspace.Family{
		keyspace.FamilyNil,
		keyspace.FamilyBool,
		keyspace.FamilyInteger,
		keyspace.FamilyFloat,
		keyspace.FamilyString,
	}
	for _, family := range families {
		count := Count(input, family)
		if count == 0 {
			t.Fatalf("literal family %d has no authored values", family)
		}
		for index := 0; index < count; index++ {
			term := keyspace.MakeTerm(family, uint32(index+1))
			gotFamily, value, ok := Literal(input, term)
			if !ok || gotFamily != family {
				t.Fatalf("Literal(%d, %d) = %d/%+v/%v", family, index, gotFamily, value, ok)
			}
			if family == keyspace.FamilyNil {
				if value != (keyspace.LiteralValue{}) {
					t.Fatalf("nil literal payload = %+v", value)
				}
				continue
			}
			wantKind := map[keyspace.Family]keyspace.LiteralKind{
				keyspace.FamilyBool:    keyspace.LiteralBool,
				keyspace.FamilyInteger: keyspace.LiteralInteger,
				keyspace.FamilyFloat:   keyspace.LiteralFloat,
				keyspace.FamilyString:  keyspace.LiteralString,
			}[family]
			if value.Kind != wantKind {
				t.Fatalf("literal family %d kind = %d, want %d", family, value.Kind, wantKind)
			}
		}
		if gotFamily, value, ok := Literal(input, keyspace.MakeTerm(family, uint32(count+1))); ok || gotFamily != family {
			t.Fatalf("out-of-bounds Literal(%d) = %d/%+v/%v", family, gotFamily, value, ok)
		}
	}
}

func TestLiteralRejectsNonLiteralFamiliesAndMalformedTerms(t *testing.T) {
	input := valueSourceLawProgram(t)
	typeValues := input.Flow().Authored().TypeValues()
	if term, ok := typeValues.At(0); ok {
		if family, value, literalOK := Literal(input, term); literalOK || family != keyspace.FamilyInvalid || value != (keyspace.LiteralValue{}) {
			t.Fatalf("TypeValue Literal(%08x) = %d/%+v/%v", uint32(term), family, value, literalOK)
		}
	}
	for _, term := range []keyspace.Term{
		0,
		keyspace.Term(1),
		keyspace.MakeTerm(keyspace.FamilyCount, 1),
		keyspace.MakeTerm(keyspace.FamilyString, 1<<24),
	} {
		if family, value, ok := Literal(input, term); ok || family != keyspace.FamilyInvalid || value != (keyspace.LiteralValue{}) {
			t.Fatalf("malformed Literal(%08x) = %d/%+v/%v", uint32(term), family, value, ok)
		}
	}
	if family, value, ok := Literal(nil, keyspace.MakeTerm(keyspace.FamilyString, 1)); ok || family != keyspace.FamilyInvalid || value != (keyspace.LiteralValue{}) {
		t.Fatalf("nil-input Literal = %d/%+v/%v", family, value, ok)
	}
}
