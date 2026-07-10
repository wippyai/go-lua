package typeoperator

import "github.com/wippyai/go-lua/analysis/type/typ"

func arithmeticBinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	if result, ok := arithmeticPrimitiveResult(left, op, right); ok {
		return result, true
	}
	if name, ok := arithmeticBinaryMetamethodName(op); ok {
		return binaryMetamethodReturn(left, name, right)
	}
	return nil, false
}

func arithmeticPrimitiveResult(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	switch op {
	case "+", "-", "*", "%":
		if isIntegerish(left) && isIntegerish(right) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case "/", "^":
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case "//":
		if isIntegerish(left) && isIntegerish(right) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case "&", "|", "~", "<<", ">>":
		if isIntegerConvertible(left) && isIntegerConvertible(right) {
			return typ.Integer, true
		}
	}
	return nil, false
}

func arithmeticBinaryMetamethodName(op string) (string, bool) {
	switch op {
	case "+":
		return "__add", true
	case "-":
		return "__sub", true
	case "*":
		return "__mul", true
	case "/":
		return "__div", true
	case "//":
		return "__idiv", true
	case "%":
		return "__mod", true
	case "^":
		return "__pow", true
	case "&":
		return "__band", true
	case "|":
		return "__bor", true
	case "~":
		return "__bxor", true
	case "<<":
		return "__shl", true
	case ">>":
		return "__shr", true
	default:
		return "", false
	}
}
