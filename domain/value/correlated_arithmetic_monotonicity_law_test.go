package value

import (
	"strconv"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/value/arithmetic/resultpolicy"
)

// TestArithmeticTransferIsMonotone states the value binary-arithmetic
// transfer law over a sealed finite atom universe: whenever both operands
// grow in the Value order, the staged result grows too. The operand universe
// contains only authored literals; every Add/IDiv result reachable from it is
// sealed in the owner schema, so refusal is an invariant violation here.
func TestArithmeticTransferIsMonotone(t *testing.T) {
	schema, atoms := monotoneArithmeticSchema(t)
	universe := arithmeticValueUniverse(t, schema, atoms)
	for _, op := range []flowkind.BinaryOp{flowkind.BinaryAdd, flowkind.BinaryIDiv} {
		candidate := arithmeticCandidateForLaw(schema, op, resultpolicy.ClosedImage(
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0},
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1},
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 2},
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3},
			keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 4},
		))
		stage := func(left, right Value) Value {
			t.Helper()
			result, ok := schema.ApplyArithmetic(candidate, left, right)
			if !ok {
				t.Fatalf("op %d refused sealed operand pair", op)
			}
			return result
		}
		for leftIndex, left := range universe {
			for leftAbove := range universe {
				if !schema.LessOrEq(left, universe[leftAbove]) {
					continue
				}
				for rightIndex, right := range universe {
					for rightAbove := range universe {
						if !schema.LessOrEq(right, universe[rightAbove]) {
							continue
						}
						below := stage(left, right)
						above := stage(universe[leftAbove], universe[rightAbove])
						if !schema.LessOrEq(below, above) {
							t.Fatalf("op %d: stage(u%d,u%d)=%s is not below stage(u%d,u%d)=%s",
								op, leftIndex, rightIndex, describeArithmeticValue(schema, below),
								leftAbove, rightAbove, describeArithmeticValue(schema, above))
						}
					}
				}
			}
		}
	}
	exact := 0
	add := arithmeticCandidateForLaw(schema, flowkind.BinaryAdd, resultpolicy.ClosedImage(
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0},
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1},
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 2},
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 3},
		keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 4},
	))
	for _, left := range universe {
		for _, right := range universe {
			result, ok := schema.ApplyArithmetic(add, left, right)
			if ok && !schema.Equal(result, schema.Bottom()) && !result.IsTop() {
				exact++
			}
		}
	}
	if exact == 0 {
		t.Fatal("no operand pair evaluated to an exact result: the law is vacuous")
	}
}

// monotoneArithmeticSchema seals three authored numeric literals and the two
// additional integer sums reachable from their complete finite operand
// universe. The operand universe itself intentionally excludes those
// computed atoms; integer division by zero remains the one genuine trap.
func monotoneArithmeticSchema(t testing.TB) (*Schema, []atomRow) {
	t.Helper()
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: 32}
	atoms := make([]atomRow, 0, 3)
	for _, literal := range []keyspace.LiteralValue{integer(0), integer(1), integer(2)} {
		schema.exactKeys[literal] = literal
		row := atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: literal, hasKey: true}
		if schema.addAtom(row) == 0 {
			t.Fatal("source literal atom")
		}
		atoms = append(atoms, row)
	}
	for _, literal := range []keyspace.LiteralValue{integer(3), integer(4)} {
		computed := atomRow{kind: atomComputedLiteral, runtime: runtimekind.Number, key: literal}
		if schema.addAtom(computed) == 0 {
			t.Fatal("computed result atom")
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	return schema, atoms
}

// arithmeticValueUniverse enumerates every relation over the authored operand
// atoms, from Bottom through the authored union, and closes with Top.
func arithmeticValueUniverse(t testing.TB, schema *Schema, rows []atomRow) []Value {
	t.Helper()
	universe := make([]Value, 0, 1<<len(rows)+1)
	for mask := 0; mask < 1<<len(rows); mask++ {
		atoms := make([]Atom, 0, len(rows))
		for index, row := range rows {
			if mask&(1<<index) == 0 {
				continue
			}
			atoms = append(atoms, Atom{schema: schema, id: schema.atomByRow[row]})
		}
		value, ok := schema.Alternatives(atoms...)
		if !ok {
			t.Fatalf("alternatives for mask %d", mask)
		}
		universe = append(universe, value)
	}
	return append(universe, schema.Top())
}

func describeArithmeticValue(schema *Schema, value Value) string {
	switch {
	case value.IsTop():
		return "Top"
	case schema.Equal(value, schema.Bottom()):
		return "Bottom"
	default:
		scalar, ok := schema.ExactScalar(value)
		literal, literalOK := scalar.Literal()
		if ok && literalOK {
			return "exact " + strconv.FormatInt(literal.Integer, 10)
		}
		return "union"
	}
}
