package value

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// ApplyArithmetic evaluates two singleton exact numeric alternatives.
// Program scalar semantics is the sole concrete arithmetic authority. Value
// translates its result into an already-sealed atom; it never mints hot
// alternatives.
//
// Every operand pair this evaluator cannot decide - an operand that is not an
// exact numeric singleton, or an exact result with no sealed atom - answers
// Top, the over-approximation that admits every alternative. Answering Bottom
// for an undecided pair would both deny reachable alternatives and break
// monotonicity, since a union operand would then evaluate below its own
// singleton members.
//
// Bottom is answered in exactly two cases, both of which keep the transfer
// monotone. Arithmetic is strict, so an operand with no reachable alternative
// yields a result with none. The other is the arithmetic trap: exact numeric
// operands whose Lua arithmetic is undefined, so evaluation cannot reach a
// concrete result.
func (schema *Schema) ApplyArithmetic(left, right Value, op flowkind.BinaryOp) (Value, bool) {
	if schema == nil || !schema.owns(left) || !schema.owns(right) || !flowkind.IsBinaryArithmetic(op) {
		return Value{}, false
	}
	if schema.Equal(left, schema.Bottom()) || schema.Equal(right, schema.Bottom()) {
		return schema.Bottom(), true
	}
	leftScalar, leftOK := schema.ExactScalar(left)
	rightScalar, rightOK := schema.ExactScalar(right)
	leftLiteral, leftLiteralOK := leftScalar.Literal()
	rightLiteral, rightLiteralOK := rightScalar.Literal()
	if !leftOK || !rightOK || !leftLiteralOK || !rightLiteralOK ||
		(leftLiteral.Kind != keyspace.LiteralInteger && leftLiteral.Kind != keyspace.LiteralFloat) ||
		(rightLiteral.Kind != keyspace.LiteralInteger && rightLiteral.Kind != keyspace.LiteralFloat) {
		return schema.Top(), true
	}
	result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, op)
	if !resultOK {
		return schema.Bottom(), true
	}
	atom := schema.atomForExactArithmetic(result)
	if atom == 0 {
		return schema.Top(), true
	}
	return schema.Singleton(Atom{schema: schema, id: atom})
}

func (schema *Schema) atomForExactArithmetic(literal keyspace.LiteralValue) uint32 {
	if schema == nil {
		return 0
	}
	if atom := schema.atomByRow[atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}]; atom != 0 {
		return atom
	}
	return schema.atomByRow[atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: literal}]
}

func (schema *valueBuilder) sealComputedArithmeticAtoms() bool {
	if schema == nil || schema.artifacts == nil {
		return false
	}
	for _, mount := range schema.artifacts {
		program := mount.Program()
		if !program.Available() {
			return false
		}
		count, published := program.ExactScalarSummaryCount()
		if !published {
			return false
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.ExactScalarSummaryAt(index)
			coldLiteral, literalOK := row.Literal()
			literal := keyspace.LiteralValue{Kind: keyspace.LiteralKind(coldLiteral.Kind), Integer: coldLiteral.Integer, FloatBits: coldLiteral.FloatBits}
			if !rowOK || !literalOK ||
				(literal.Kind != keyspace.LiteralInteger && literal.Kind != keyspace.LiteralFloat) {
				return false
			}
			if row.Role() != programschema.ExactScalarSummaryResult {
				continue
			}
			if schema.atomForExactArithmetic(literal) != 0 {
				continue
			}
			if schema.addAtom(atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: literal}) == 0 {
				return false
			}
		}
	}
	return true
}
