package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

func TestCompareEqualityUsesOwnedExactScalarsAndNilability(t *testing.T) {
	stringName := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "string"}
	numberName := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "number"}
	integerOne := keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}
	floatOne := keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: 0x3ff0000000000000}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 16}
	for _, literal := range []keyspace.LiteralValue{stringName, numberName, integerOne, floatOne} {
		schema.exactKeys[literal] = literal
	}
	if schema.addAtom(atomRow{kind: atomNil}) == 0 || schema.addAtom(atomRow{kind: atomFalse}) == 0 || schema.addAtom(atomRow{kind: atomTrue}) == 0 || schema.addAtom(atomRow{kind: atomPrimitive, runtime: runtimekind.String}) == 0 {
		t.Fatal("test schema atoms unavailable")
	}
	for _, literal := range []keyspace.LiteralValue{stringName, numberName} {
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.String, key: literal, hasKey: true}) == 0 {
			t.Fatal("string literal atom unavailable")
		}
	}
	for _, literal := range []keyspace.LiteralValue{integerOne, floatOne} {
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("numeric literal atom unavailable")
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	nilValue, nilOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomNil}]})
	stringValue, stringOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.String}]})
	optional, optionalOK := schema.Join(nilValue, stringValue)
	if !nilOK || !stringOK || !optionalOK {
		t.Fatal("test values unavailable")
	}

	equalNil, ok := schema.CompareEquality(nilValue, nilValue, false)
	if !ok || schema.Truthiness(equalNil) != TruthTrue {
		t.Fatalf("nil == nil truth = %v/%v, want true", schema.Truthiness(equalNil), ok)
	}
	notEqualNil, ok := schema.CompareEquality(nilValue, stringValue, true)
	if !ok || schema.Truthiness(notEqualNil) != TruthTrue {
		t.Fatalf("nil ~= string truth = %v/%v, want true", schema.Truthiness(notEqualNil), ok)
	}
	unknown, ok := schema.CompareEquality(optional, nilValue, true)
	if !ok || schema.Truthiness(unknown) != TruthFalse|TruthTrue {
		t.Fatalf("optional ~= nil truth = %v/%v, want both", schema.Truthiness(unknown), ok)
	}
	stringNameValue := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.String, key: stringName, hasKey: true})
	numberNameValue := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.String, key: numberName, hasKey: true})
	differentNames, ok := schema.CompareEquality(stringNameValue, numberNameValue, true)
	if !ok || schema.Truthiness(differentNames) != TruthTrue {
		t.Fatalf("string ~= number truth = %v/%v, want true", schema.Truthiness(differentNames), ok)
	}
	integerValue := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integerOne, hasKey: true})
	floatValue := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: floatOne, hasKey: true})
	numericEqual, ok := schema.CompareEquality(integerValue, floatValue, false)
	if !ok || schema.Truthiness(numericEqual) != TruthTrue {
		t.Fatalf("integer 1 == float 1 truth = %v/%v, want true", schema.Truthiness(numericEqual), ok)
	}
	if _, ok := (&Schema{}).CompareEquality(nilValue, nilValue, false); ok {
		t.Fatal("foreign schema accepted equality operands")
	}
}

func TestFilterPresencePartitionsNilWithoutDiscardingFalse(t *testing.T) {
	schema := &Schema{atomByRow: make(map[atomRow]uint32), potential: 8}
	if schema.addAtom(atomRow{kind: atomNil}) == 0 || schema.addAtom(atomRow{kind: atomFalse}) == 0 || schema.addAtom(atomRow{kind: atomTrue}) == 0 || schema.addAtom(atomRow{kind: atomPrimitive, runtime: runtimekind.String}) == 0 {
		t.Fatal("test schema atoms unavailable")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	nilValue, nilOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomNil}]})
	falseValue, falseOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomFalse}]})
	stringValue, stringOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.String}]})
	presentAlternatives, presentOK := schema.Join(falseValue, stringValue)
	optional, optionalOK := schema.Join(nilValue, presentAlternatives)
	if !nilOK || !falseOK || !stringOK || !presentOK || !optionalOK {
		t.Fatal("test values unavailable")
	}

	present, presentFilterOK := schema.FilterPresence(optional, true)
	absent, absentFilterOK := schema.FilterPresence(optional, false)
	if !presentFilterOK || !absentFilterOK || !schema.Equal(present, presentAlternatives) || !schema.Equal(absent, nilValue) {
		t.Fatalf("nilability partition mismatch: present=%v absent=%v", presentFilterOK, absentFilterOK)
	}
	nonNilAbsent, nonNilAbsentOK := schema.FilterPresence(stringValue, false)
	if !nonNilAbsentOK || !schema.Equal(nonNilAbsent, schema.Bottom()) {
		t.Fatal("absent arm retained a definitely-present value")
	}
	if _, ok := (&Schema{}).FilterPresence(optional, true); ok {
		t.Fatal("foreign schema accepted presence refinement")
	}
}
