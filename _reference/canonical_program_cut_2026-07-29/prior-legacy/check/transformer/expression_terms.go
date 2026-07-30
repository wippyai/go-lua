package transformer

import (
	"fmt"
)

// ScalarUnaryValue retains one pure Lua scalar unary operation. The operand
// remains symbolic until specialization; sourcevalue owns truthiness.
func (a *Arena) ScalarUnaryValue(operator string, operand ValueTerm) (ValueTerm, bool) {
	if a == nil || !isPureUnaryOperator(operator) || operand == 0 || int(operand) >= len(a.values) {
		return 0, false
	}
	return a.internValue(valueNode{op: valueUnaryOperation, operator: operator, args: []ValueTerm{operand}}), true
}

func isPureUnaryOperator(operator string) bool {
	switch operator {
	case "not", "#", "-", "~":
		return true
	default:
		return false
	}
}

func canonicalScalarUnaryValue(operator, operand string) string {
	if !isPureUnaryOperator(operator) {
		return "_"
	}
	return fmt.Sprintf("u%s(%s)", operator, operand)
}

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
	if !isPureBinaryOperator(operator) {
		return 0, false
	}
	return a.scalarBinaryValue(operator, left, right), true
}

func isPureBinaryOperator(operator string) bool {
	switch operator {
	case "+", "-", "*", "/", "//", "%", "^", "&", "|", "~", "<<", ">>",
		"==", "~=", "<", "<=", ">", ">=", "and", "or":
		return true
	default:
		return false
	}
}

func (a *Arena) scalarBinaryValue(operator string, left, right ValueTerm) ValueTerm {
	if !isPureBinaryOperator(operator) || left == 0 || right == 0 {
		return 0
	}
	if operator == "==" || operator == "~=" {
		leftKey, rightKey := a.canonicalValue(left), a.canonicalValue(right)
		if rightKey < leftKey || rightKey == leftKey && right < left {
			left, right = right, left
		}
	}
	return a.internValue(valueNode{op: valueBinaryOperation, operator: operator, args: []ValueTerm{left, right}})
}

func canonicalScalarBinaryValue(operator, left, right string) string {
	if !isPureBinaryOperator(operator) {
		return "_"
	}
	return fmt.Sprintf("b%s(%s,%s)", operator, left, right)
}
