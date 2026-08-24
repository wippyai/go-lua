package value

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
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

func TestOrderValueAuthenticatesOwnerAndMapsExplicitBottomToNoCandidate(t *testing.T) {
	threeLiteral := keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3}
	fiveLiteral := keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 5}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 8}
	for _, literal := range []keyspace.LiteralValue{threeLiteral, fiveLiteral} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("test literal atom unavailable")
		}
	}
	if schema.addAtom(atomRow{kind: atomFalse}) == 0 || schema.addAtom(atomRow{kind: atomTrue}) == 0 {
		t.Fatal("test Boolean atoms unavailable")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	three := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: threeLiteral, hasKey: true})
	five := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: fiveLiteral, hasKey: true})
	candidate := BinaryOrder{
		schema:  schema,
		key:     computationKey{module: identity.ContentID{1}, occurrence: identity.ContentID{2}},
		content: identity.ContentID{3},
		op:      flowkind.BinaryLess,
	}
	result, outcome := OrderValue(candidate, three, five)
	if outcome != structure.Concrete || schema.Truthiness(result) != TruthTrue {
		t.Fatalf("OrderValue outcome/result = %v/%v, want Concrete/true", outcome, schema.Truthiness(result))
	}
	_, bottomOutcome := OrderValue(candidate, schema.Bottom(), five)
	if bottomOutcome != structure.NoCandidate {
		t.Fatalf("OrderValue Bottom outcome = %v, want NoCandidate", bottomOutcome)
	}
	foreign := *schema
	if _, foreignOutcome := OrderValue(candidate, Value{schema: &foreign, top: true}, five); foreignOutcome != structure.Refuse {
		t.Fatalf("foreign Value outcome = %v, want Refuse", foreignOutcome)
	}
	if _, missingOutcome := OrderValue(candidate, Value{}, five); missingOutcome != structure.Refuse {
		t.Fatalf("missing Value outcome = %v, want executor-owned Refuse at fold boundary", missingOutcome)
	}
	malformed := candidate
	malformed.op = flowkind.BinaryEqual
	if _, malformedOutcome := OrderValue(malformed, three, five); malformedOutcome != structure.Refuse {
		t.Fatalf("malformed candidate outcome = %v, want Refuse", malformedOutcome)
	}
}
