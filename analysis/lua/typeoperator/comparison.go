package typeoperator

import "github.com/wippyai/go-lua/analysis/type/typ"

func comparisonBinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	if result, ok := comparisonPrimitiveResult(left, right); ok {
		return result, true
	}
	name, ok := comparisonMetamethodName(op)
	if !ok {
		return nil, false
	}
	if op == ">" || op == ">=" {
		return binaryMetamethodReturn(right, name, left)
	}
	return binaryMetamethodReturn(left, name, right)
}

func comparisonPrimitiveResult(left, right typ.Type) (typ.Type, bool) {
	if (isNumericType(left) && isNumericType(right)) || (isStringLike(left) && isStringLike(right)) {
		return typ.Boolean, true
	}
	return nil, false
}

func comparisonMetamethodName(op string) (string, bool) {
	switch op {
	case "<", ">":
		return "__lt", true
	case "<=", ">=":
		return "__le", true
	default:
		return "", false
	}
}
