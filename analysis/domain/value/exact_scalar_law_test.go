package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestExactScalarRequiresOneOwnedExactAlternativeLaw(t *testing.T) {
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 16}
	rows := []atomRow{
		{kind: atomNil},
		{kind: atomFalse},
		{kind: atomTrue},
		{kind: atomLiteral, runtime: runtimekind.Number, key: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3}, hasKey: true},
		{kind: atomPrimitive, runtime: runtimekind.Number},
	}
	schema.exactKeys[rows[3].key] = rows[3].key
	for _, row := range rows {
		if schema.addAtom(row) == 0 {
			t.Fatal("add exact scalar atom")
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	wants := []struct {
		row     atomRow
		kind    ExactScalarKind
		literal keyspace.LiteralValue
		litOK   bool
	}{
		{row: rows[0], kind: ExactScalarNil},
		{row: rows[1], kind: ExactScalarBoolean, literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool}, litOK: true},
		{row: rows[2], kind: ExactScalarBoolean, literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true}, litOK: true},
		{row: rows[3], kind: ExactScalarLiteral, literal: rows[3].key, litOK: true},
	}
	for _, want := range wants {
		value, valueOK := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[want.row]})
		scalar, scalarOK := schema.ExactScalar(value)
		literal, literalOK := scalar.Literal()
		if !valueOK || !scalarOK || scalar.Kind() != want.kind || literalOK != want.litOK || literalOK && literal != want.literal {
			t.Fatalf("ExactScalar(%v) = kind:%d literal:%+v/%v, want %d %+v/%v", want.row.kind, scalar.Kind(), literal, literalOK, want.kind, want.literal, want.litOK)
		}
	}
	opaque, _ := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[rows[4]]})
	mixed, _ := schema.Alternatives(Atom{schema: schema, id: schema.atomByRow[rows[1]]}, Atom{schema: schema, id: schema.atomByRow[rows[2]]})
	foreign := *schema
	if _, ok := schema.ExactScalar(opaque); ok {
		t.Fatal("opaque scalar became exact")
	}
	if _, ok := schema.ExactScalar(mixed); ok {
		t.Fatal("multi-alternative scalar became exact")
	}
	if _, ok := schema.ExactScalar(schema.Top()); ok {
		t.Fatal("Top became exact")
	}
	if _, ok := foreign.ExactScalar(wantsValue(t, schema, rows[3])); ok {
		t.Fatal("foreign equal-content schema accepted exact scalar")
	}
}

func wantsValue(t testing.TB, schema *Schema, row atomRow) Value {
	t.Helper()
	value, ok := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[row]})
	if !ok {
		t.Fatal("singleton")
	}
	return value
}
