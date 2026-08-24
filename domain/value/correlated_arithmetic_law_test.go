package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

func TestApplyArithmeticUsesProgramSemanticsAndSealedResultAtoms(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(10), integer(5), integer(0)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("source literal atom")
		}
	}
	resultLiteral := integer(15)
	if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: resultLiteral}) == 0 {
		t.Fatal("computed result atom")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	left := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(10), hasKey: true})
	right := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(5), hasKey: true})
	result, ok := schema.ApplyArithmetic(left, right, flowkind.BinaryAdd)
	scalar, scalarOK := schema.ExactScalar(result)
	literal, literalOK := scalar.Literal()
	if !ok || !scalarOK || !literalOK || literal != resultLiteral {
		t.Fatalf("ApplyArithmetic(10,5,+) = %+v/%v scalar=%+v/%v literal=%+v/%v", result, ok, scalar, scalarOK, literal, literalOK)
	}
	if _, keyOK := (Atom{schema: schema, id: schema.atomForExactArithmetic(resultLiteral)}).ExactKey(); keyOK {
		t.Fatal("computed arithmetic result fabricated a Link key")
	}
	reused, reusedOK := schema.ApplyArithmetic(right, right, flowkind.BinaryAdd)
	reusedScalar, reusedScalarOK := schema.ExactScalar(reused)
	reusedLiteral, reusedLiteralOK := reusedScalar.Literal()
	if !reusedOK || !reusedScalarOK || !reusedLiteralOK || reusedLiteral != integer(10) {
		t.Fatal("arithmetic result did not reuse an authored literal atom")
	}
	missing, missingOK := schema.ApplyArithmetic(right, right, flowkind.BinaryMul)
	if !missingOK || !missing.IsTop() {
		t.Fatal("exact result without a sealed atom denied its reachable alternatives")
	}
	if schema.atomForExactArithmetic(integer(25)) != 0 {
		t.Fatal("unsealed exact result was fabricated")
	}
	mixed, _ := schema.Alternatives(
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(10))},
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(5))},
	)
	unknown, unknownOK := schema.ApplyArithmetic(mixed, right, flowkind.BinaryAdd)
	if !unknownOK || !unknown.IsTop() {
		t.Fatal("mixed input produced an exact arithmetic result")
	}
	if _, scalarOK := schema.ExactScalar(unknown); scalarOK {
		t.Fatal("undecided arithmetic claimed an exact scalar")
	}
	strict, strictOK := schema.ApplyArithmetic(schema.Bottom(), right, flowkind.BinaryAdd)
	if !strictOK || !schema.Equal(strict, schema.Bottom()) {
		t.Fatal("arithmetic over an unreachable operand invented a reachable result")
	}
	trap, trapOK := schema.ApplyArithmetic(left, wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(0), hasKey: true}), flowkind.BinaryIDiv)
	if !trapOK || !schema.Equal(trap, schema.Bottom()) {
		t.Fatal("integer division by zero kept a reachable alternative")
	}
	foreign := *schema
	if _, ok := foreign.ApplyArithmetic(left, right, flowkind.BinaryAdd); ok {
		t.Fatal("foreign equal-content Value owner accepted operands")
	}
	if _, ok := schema.ApplyArithmetic(left, right, flowkind.BinaryEqual); ok {
		t.Fatal("non-arithmetic operator accepted")
	}
}

func TestArithmeticValueOwnsTheCompleteReductionOutcome(t *testing.T) {
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	for _, literal := range []keyspace.LiteralValue{integer(10), integer(5)} {
		schema.exactKeys[literal] = literal
		if schema.addAtom(atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}) == 0 {
			t.Fatal("source literal atom")
		}
	}
	if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: integer(15)}) == 0 {
		t.Fatal("computed result atom")
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	candidate := BinaryArithmetic{
		schema:  schema,
		key:     computationKey{module: identity.ContentID{1}, occurrence: identity.ContentID{2}},
		content: identity.ContentID{3},
		op:      flowkind.BinaryAdd,
	}
	left := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(10), hasKey: true})
	right := wantsValue(t, schema, atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: integer(5), hasKey: true})

	result, outcome := ArithmeticValue(candidate, left, right)
	if outcome != structure.Concrete {
		t.Fatalf("ArithmeticValue outcome = %v, want Concrete", outcome)
	}
	scalar, scalarOK := schema.ExactScalar(result)
	literal, literalOK := scalar.Literal()
	if !scalarOK || !literalOK || literal != integer(15) {
		t.Fatalf("ArithmeticValue result = %+v/%v literal=%+v/%v, want integer 15", scalar, scalarOK, literal, literalOK)
	}

	foreign := *schema
	if _, outcome := ArithmeticValue(candidate, Value{schema: &foreign, top: true}, right); outcome != structure.Refuse {
		t.Fatalf("foreign operand outcome = %v, want Refuse", outcome)
	}
}
