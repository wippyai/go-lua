package value

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestCompareOrderRetainsExactLiteralsAndConservativeUnknowns(t *testing.T) {
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 64}
	rows := []atomRow{
		{kind: atomFalse},
		{kind: atomTrue},
		{kind: atomLiteral, runtime: runtimekind.Number, key: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3}, hasKey: true},
		{kind: atomLiteral, runtime: runtimekind.Number, key: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 5}, hasKey: true},
		{kind: atomLiteral, runtime: runtimekind.Number, key: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(3.5)}, hasKey: true},
		{kind: atomLiteral, runtime: runtimekind.String, key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "a"}, hasKey: true},
		{kind: atomLiteral, runtime: runtimekind.String, key: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "b"}, hasKey: true},
		{kind: atomPrimitive, runtime: runtimekind.Number},
		{kind: atomNaN, runtime: runtimekind.Number},
		{kind: atomOpaqueKind, runtime: runtimekind.Boolean},
	}
	for _, row := range rows {
		if row.kind == atomLiteral {
			schema.exactKeys[row.key] = row.key
		}
	}
	for _, row := range rows {
		if schema.addAtom(row) == 0 {
			t.Fatalf("test atom unavailable: %+v", row)
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	value := func(row atomRow) Value {
		result, ok := schema.Singleton(Atom{schema: schema, id: schema.atomByRow[row]})
		if !ok {
			t.Fatalf("singleton unavailable: %+v", row)
		}
		return result
	}
	three := value(rows[2])
	five := value(rows[3])
	threePointFive := value(rows[4])
	a := value(rows[5])
	b := value(rows[6])
	unknownNumber := value(rows[7])
	nan := value(rows[8])
	boolean := value(rows[9])

	assertTruth := func(name string, left, right Value, op flowkind.BinaryOp, want Truth) {
		t.Helper()
		result, ok := schema.CompareOrder(left, right, op)
		if !ok || schema.Truthiness(result) != want {
			t.Fatalf("%s truth = %v/%v, want %v", name, schema.Truthiness(result), ok, want)
		}
	}
	assertTruth("3 > 5", three, five, flowkind.BinaryGreater, TruthFalse)
	assertTruth("3 < 5", three, five, flowkind.BinaryLess, TruthTrue)
	assertTruth("3.5 >= 3", threePointFive, three, flowkind.BinaryGreaterEqual, TruthTrue)
	assertTruth("a <= b", a, b, flowkind.BinaryLessEqual, TruthTrue)
	assertTruth("unknown number", unknownNumber, five, flowkind.BinaryGreater, TruthFalse|TruthTrue)
	assertTruth("NaN < 5", nan, five, flowkind.BinaryLess, TruthFalse)
	assertTruth("Top order", schema.Top(), five, flowkind.BinaryGreater, TruthFalse|TruthTrue)
	assertTruth("unsupported boolean", boolean, boolean, flowkind.BinaryLess, TruthNone)

	mixed, mixedOK := schema.Join(three, five)
	if !mixedOK {
		t.Fatal("mixed numeric relation")
	}
	assertTruth("mixed exact order", mixed, five, flowkind.BinaryGreaterEqual, TruthFalse|TruthTrue)
	if _, ok := schema.CompareOrder(three, five, flowkind.BinaryEqual); ok {
		t.Fatal("equality operator entered order relation")
	}
	if _, ok := (&Schema{}).CompareOrder(three, five, flowkind.BinaryLess); ok {
		t.Fatal("foreign schema accepted order operands")
	}
}
