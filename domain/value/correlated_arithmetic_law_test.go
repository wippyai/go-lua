package value

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

func TestApplyArithmeticUsesProgramSemanticsAndSealedResultAtoms(t *testing.T) {
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
	if !missingOK || !schema.Equal(missing, schema.Bottom()) {
		t.Fatal("unsealed exact result was fabricated")
	}
	mixed, _ := schema.Alternatives(
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(10))},
		Atom{schema: schema, id: schema.atomForExactArithmetic(integer(5))},
	)
	unknown, unknownOK := schema.ApplyArithmetic(mixed, right, flowkind.BinaryAdd)
	if !unknownOK || !schema.Equal(unknown, schema.Bottom()) {
		t.Fatal("mixed input produced an exact arithmetic result")
	}
	foreign := *schema
	if _, ok := foreign.ApplyArithmetic(left, right, flowkind.BinaryAdd); ok {
		t.Fatal("foreign equal-content Value owner accepted operands")
	}
	if _, ok := schema.ApplyArithmetic(left, right, flowkind.BinaryEqual); ok {
		t.Fatal("non-arithmetic operator accepted")
	}
}
