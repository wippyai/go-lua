package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

// ScalarBinaryValue retains one of the pure scalar operations needed by a
// symbolic function relation. The operands remain terms until specialization;
// this constructor never manufactures a product.Value or guesses Lua
// truthiness. Equality is commutative and receives a canonical operand order.
// Logical and/or retain source order because their result is one of the
// operands, not a boolean.
func (a *Arena) ScalarBinaryValue(operator string, left, right ValueTerm) (ValueTerm, bool) {
	if a == nil || left == 0 || right == 0 || int(left) >= len(a.values) || int(right) >= len(a.values) {
		return 0, false
	}
	var op valueOp
	switch operator {
	case "==":
		op = valueScalarEqual
	case "~=":
		op = valueScalarNotEqual
	case "and":
		op = valueScalarAnd
	case "or":
		op = valueScalarOr
	default:
		return 0, false
	}
	return a.scalarBinaryValue(op, left, right), true
}

func (a *Arena) scalarBinaryValue(op valueOp, left, right ValueTerm) ValueTerm {
	if !isScalarBinaryValueOp(op) || left == 0 || right == 0 {
		return 0
	}
	if op == valueScalarEqual || op == valueScalarNotEqual {
		leftKey, rightKey := a.canonicalValue(left), a.canonicalValue(right)
		if rightKey < leftKey || rightKey == leftKey && right < left {
			left, right = right, left
		}
	}
	return a.internValue(valueNode{op: op, args: []ValueTerm{left, right}})
}

func isScalarBinaryValueOp(op valueOp) bool {
	return op >= valueScalarEqual && op <= valueScalarOr
}

func scalarBinaryOperator(op valueOp) (string, bool) {
	switch op {
	case valueScalarEqual:
		return "==", true
	case valueScalarNotEqual:
		return "~=", true
	case valueScalarAnd:
		return "and", true
	case valueScalarOr:
		return "or", true
	default:
		return "", false
	}
}

func (a *Arena) evalScalarBinaryValue(op valueOp, args []ValueTerm, cursor BindingCursor, context SpecializationContext) (product.Value, bool) {
	operator, ok := scalarBinaryOperator(op)
	if !ok || len(args) != 2 {
		return product.Value{}, false
	}
	left, ok := a.evalValue(args[0], cursor, context)
	if !ok {
		return product.Value{}, false
	}
	right, ok := a.evalValue(args[1], cursor, context)
	if !ok {
		return product.Value{}, false
	}
	// This is deliberately the sole semantic authority. Symbolic terms own
	// representation and substitution; Lua sourcevalue owns abstract execution.
	return luasourcevalue.BinaryOperationValue(a.reg, nil, operator, left, right)
}

func canonicalScalarBinaryValue(op valueOp, left, right string) string {
	operator, ok := scalarBinaryOperator(op)
	if !ok {
		return "_"
	}
	return fmt.Sprintf("b%s(%s,%s)", operator, left, right)
}
