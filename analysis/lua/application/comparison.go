package application

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
)

func comparisonBinaryResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
	if result, ok := comparisonPrimitiveResult(left, right); ok {
		return result, true
	}
	law, ok := Binary(op)
	if !ok || law.Meta == MetaAbsent {
		return nil, false
	}
	if law.ReverseOperands {
		return binaryMetamethodReturn(right, law.Meta, left)
	}
	return binaryMetamethodReturn(left, law.Meta, right)
}

func comparisonPrimitiveResult(left, right typ.Type) (typ.Type, bool) {
	if (isNumericType(left) && isNumericType(right)) || (isStringLike(left) && isStringLike(right)) {
		return typ.Boolean, true
	}
	return nil, false
}
