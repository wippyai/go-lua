package value

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// NumericOverflow is Lua's closed overflow discipline for numeric arithmetic:
// what an operator does to the representation of its result once the
// representations of its operands are known. It belongs beside Value's
// arithmetic because it is numeric semantics rather than a rendering choice,
// and its spellings are stated here once so every consumer names the same
// discipline the same way.
type NumericOverflow uint8

const (
	NumericOverflowInvalid NumericOverflow = iota
	// NumericOverflowClosedInteger keeps the operation inside the integer
	// representation: no operand pair it admits can leave that representation.
	NumericOverflowClosedInteger
	// NumericOverflowPromoteIntegerToNumber evaluates the operation on integers
	// and leaves the wider number representation as the result's, because an
	// exceeded integer range is answered by promotion rather than by wrapping.
	NumericOverflowPromoteIntegerToNumber
	// NumericOverflowIEEE754 evaluates the operation in floating point and
	// carries IEEE-754's own overflow behavior.
	NumericOverflowIEEE754
)

func (overflow NumericOverflow) Valid() bool {
	return overflow >= NumericOverflowClosedInteger && overflow <= NumericOverflowIEEE754
}

func (overflow NumericOverflow) String() string {
	switch overflow {
	case NumericOverflowClosedInteger:
		return "closed_integer"
	case NumericOverflowPromoteIntegerToNumber:
		return "promote_integer_to_number"
	case NumericOverflowIEEE754:
		return "ieee754"
	default:
		return ""
	}
}

// numericOverflowRow is one operator's discipline: the one that governs the
// operation when every operand is integer-represented, and the one that
// governs every other operand shape.
type numericOverflowRow struct {
	integer, widened NumericOverflow
}

func (row numericOverflowRow) resolve(integer bool) (NumericOverflow, bool) {
	overflow := row.widened
	if integer {
		overflow = row.integer
	}
	return overflow, overflow.Valid()
}

// binaryNumericOverflows declares the discipline of the sealed binary
// arithmetic operator vocabulary, indexed by the operator's own ordinal. An
// operator that joins flowkind's arithmetic range without a row here holds an
// invalid discipline and fails closed rather than rendering a default.
var binaryNumericOverflows = [flowkind.BinaryPow + 1]numericOverflowRow{
	flowkind.BinaryAdd:  {integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
	flowkind.BinarySub:  {integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
	flowkind.BinaryMul:  {integer: NumericOverflowPromoteIntegerToNumber, widened: NumericOverflowIEEE754},
	flowkind.BinaryDiv:  {integer: NumericOverflowIEEE754, widened: NumericOverflowIEEE754},
	flowkind.BinaryIDiv: {integer: NumericOverflowClosedInteger, widened: NumericOverflowIEEE754},
	flowkind.BinaryMod:  {integer: NumericOverflowClosedInteger, widened: NumericOverflowIEEE754},
	flowkind.BinaryPow:  {integer: NumericOverflowIEEE754, widened: NumericOverflowIEEE754},
}

// unaryNumericOverflows declares the same discipline for the one arithmetic
// member of the sealed unary operator vocabulary.
var unaryNumericOverflows = [flowkind.UnaryNeg + 1]numericOverflowRow{
	flowkind.UnaryNeg: {integer: NumericOverflowClosedInteger, widened: NumericOverflowIEEE754},
}

// BinaryNumericOverflow states one binary arithmetic operator's overflow
// discipline over two known operand representations. Operators outside the
// arithmetic range and unknown representations have no discipline.
func BinaryNumericOverflow(op flowkind.BinaryOp, left, right programschema.NumericRepresentation) (NumericOverflow, bool) {
	if !flowkind.IsBinaryArithmetic(op) || !left.Valid() || !right.Valid() {
		return NumericOverflowInvalid, false
	}
	integer := left == programschema.NumericRepresentationInteger && right == programschema.NumericRepresentationInteger
	return binaryNumericOverflows[op].resolve(integer)
}

// UnaryNumericOverflow states the same for authored unary negation, the only
// unary operator that carries a numeric result representation.
func UnaryNumericOverflow(op flowkind.UnaryOp, operand programschema.NumericRepresentation) (NumericOverflow, bool) {
	if op != flowkind.UnaryNeg || !operand.Valid() {
		return NumericOverflowInvalid, false
	}
	return unaryNumericOverflows[op].resolve(operand == programschema.NumericRepresentationInteger)
}
