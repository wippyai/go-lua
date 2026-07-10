package typeoperator

import "github.com/wippyai/go-lua/analysis/type/typ"

func concatBinaryOp(left typ.Type, right typ.Type) (typ.Type, bool) {
	if result, ok := concatPrimitiveResult(left, right); ok {
		return result, true
	}
	return binaryMetamethodReturn(left, "__concat", right)
}

func concatPrimitiveResult(left, right typ.Type) (typ.Type, bool) {
	if isConcatOperand(left) && isConcatOperand(right) {
		return typ.String, true
	}
	return nil, false
}
