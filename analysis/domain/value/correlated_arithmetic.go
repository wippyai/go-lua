package value

import (
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// ApplyArithmetic evaluates only two singleton exact numeric alternatives.
// Program is the sole concrete Lua arithmetic authority.  Value translates
// the result into an already-sealed atom; it never mints hot alternatives.
func (schema *Schema) ApplyArithmetic(left, right Value, op flowkind.BinaryOp) (Value, bool) {
	if schema == nil || !schema.owns(left) || !schema.owns(right) || !binaryArithmeticOperator(op) {
		return Value{}, false
	}
	leftScalar, leftOK := schema.ExactScalar(left)
	rightScalar, rightOK := schema.ExactScalar(right)
	leftLiteral, leftLiteralOK := leftScalar.Literal()
	rightLiteral, rightLiteralOK := rightScalar.Literal()
	if !leftOK || !rightOK || !leftLiteralOK || !rightLiteralOK ||
		(leftLiteral.Kind != keyspace.LiteralInteger && leftLiteral.Kind != keyspace.LiteralFloat) ||
		(rightLiteral.Kind != keyspace.LiteralInteger && rightLiteral.Kind != keyspace.LiteralFloat) {
		return schema.Bottom(), true
	}
	result, resultOK := program.ExactArithmeticLiteral(leftLiteral, rightLiteral, op)
	if !resultOK {
		return schema.Bottom(), true
	}
	atom := schema.atomForExactArithmetic(result)
	if atom == 0 {
		return schema.Bottom(), true
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
		artifact := mount.Artifact()
		if artifact == nil {
			return false
		}
		for index := 0; index < artifact.ExactScalarSummaryCount(); index++ {
			row, rowOK := artifact.ExactScalarSummaryAt(index)
			literal, literalOK := row.Literal()
			if !rowOK || !literalOK ||
				(literal.Kind != keyspace.LiteralInteger && literal.Kind != keyspace.LiteralFloat) {
				return false
			}
			if row.Role() != programartifact.ExactScalarSummaryResult {
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
