package value

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// ApplyArithmetic evaluates the complete finite Cartesian product of two
// owner-issued numeric relations. Program scalar semantics is the sole
// concrete arithmetic authority. Value translates each result into an
// already-sealed atom and joins those atoms; it never mints a hot alternative
// or asks a downstream consumer to rediscover a literal.
//
// Top remains the owner-authored answer only for an unbounded input relation
// (or a finite relation containing an opaque/non-numeric atom), where no
// finite exact image is available. A finite numeric relation is different:
// every result atom is required to have been sealed by Program/Value. A
// missing result atom is a construction defect and therefore refuses closed,
// rather than widening one missing cell to Top.
//
// Bottom is answered when arithmetic has no reachable successful pair: strict
// arithmetic over an empty operand, or numeric trap pairs such as division by
// zero. Trap pairs contribute no result and never erase other valid cells.
func (schema *Schema) ApplyArithmetic(candidate BinaryArithmetic, left, right Value) (Value, bool) {
	if schema == nil || !schema.OwnsBinaryArithmetic(candidate) || !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	op := candidate.op
	if schema.Equal(left, schema.Bottom()) || schema.Equal(right, schema.Bottom()) {
		return schema.Bottom(), true
	}
	// A Top relation has no finite exact image. It is the only lawful path
	// where this evaluator widens instead of enumerating atoms.
	if left.top || right.top {
		return schema.Top(), true
	}

	closed := candidate.policy.Closed()
	leftCount, rightCount := schema.ValueAtomCount(left), schema.ValueAtomCount(right)
	var resultAtoms []Atom
	var seen map[uint32]struct{}
	if closed {
		resultAtoms = make([]Atom, 0, leftCount*rightCount)
		seen = make(map[uint32]struct{}, leftCount*rightCount)
	}
	openResult := false
	for leftIndex := 0; leftIndex < leftCount; leftIndex++ {
		leftAtom, leftOK := schema.ValueAtomAt(left, leftIndex)
		leftLiteral, leftLiteralOK := schema.numericLiteral(leftAtom)
		leftAbstractOK := schema.abstractNumericAtom(leftAtom)
		if !leftOK || !leftLiteralOK && !leftAbstractOK {
			// This finite relation is not a finite numeric arithmetic
			// domain. Its dynamic result may be any owner alternative.
			return schema.Top(), true
		}
		for rightIndex := 0; rightIndex < rightCount; rightIndex++ {
			rightAtom, rightOK := schema.ValueAtomAt(right, rightIndex)
			rightLiteral, rightLiteralOK := schema.numericLiteral(rightAtom)
			rightAbstractOK := schema.abstractNumericAtom(rightAtom)
			if !rightOK || !rightLiteralOK && !rightAbstractOK {
				return schema.Top(), true
			}
			if !leftLiteralOK || !rightLiteralOK {
				if closed {
					// Program proved this occurrence's operand images finite
					// and Value carries an abstract numeric atom for one of
					// them. The two disagree about the same expression.
					return Value{}, false
				}
				// Program declared this occurrence's exact image open. Its
				// abstract numeric operands therefore stay inside the sealed
				// numeric relation instead of widening to Value Top.
				openResult = true
				continue
			}
			result, resultOK := scalar.ExactArithmeticLiteral(leftLiteral, rightLiteral, op)
			if !resultOK {
				// Undefined numeric pairs are arithmetic traps. They have
				// no reachable result but do not invalidate another pair.
				continue
			}
			if !closed {
				// An exact pair of an open occurrence proves nothing about
				// the occurrence: a recurrence reaches this pair once and
				// another value of it on the next iteration.
				openResult = true
				continue
			}
			if !candidate.policy.Admits(result) {
				return Value{}, false
			}
			atomID := schema.atomForExactArithmetic(result)
			if atomID == 0 {
				// The owner promised a finite exact product but did not
				// seal this result atom. Refuse instead of dynamically
				// extending the atom universe or returning an invented Top.
				return Value{}, false
			}
			if _, duplicate := seen[atomID]; duplicate {
				continue
			}
			seen[atomID] = struct{}{}
			resultAtoms = append(resultAtoms, Atom{schema: schema, id: atomID})
		}
	}
	if !closed {
		if !openResult {
			return schema.Bottom(), true
		}
		return schema.ForRuntimeKinds(runtimekind.Bit(runtimekind.Number))
	}
	if len(resultAtoms) == 0 {
		return schema.Bottom(), true
	}
	return schema.Alternatives(resultAtoms...)
}

func (schema *Schema) abstractNumericAtom(atom Atom) bool {
	if schema == nil || !schema.OwnsAtom(atom) {
		return false
	}
	row := schema.atoms[atom.id-1]
	if row.runtime != runtimekind.Number || (row.kind != atomPrimitive && row.kind != atomNaN && row.kind != atomOpaqueKind) {
		return false
	}
	return true
}

// numericLiteral is the Value-owned exact numeric projection used by the
// finite arithmetic product. It accepts authored and Program-computed number
// atoms only; primitive/NaN number atoms intentionally remain opaque because
// they do not carry one concrete payload.
func (schema *Schema) numericLiteral(atom Atom) (keyspace.LiteralValue, bool) {
	if schema == nil || !schema.OwnsAtom(atom) {
		return keyspace.LiteralValue{}, false
	}
	row := schema.atoms[atom.id-1]
	if (row.kind != atomLiteral && row.kind != atomComputedLiteral) || row.runtime != runtimekind.Number {
		return keyspace.LiteralValue{}, false
	}
	if row.kind == atomLiteral && !row.hasKey || row.key.Kind != keyspace.LiteralInteger && row.key.Kind != keyspace.LiteralFloat {
		return keyspace.LiteralValue{}, false
	}
	return row.key, true
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
		program := mount.Program.Program
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
