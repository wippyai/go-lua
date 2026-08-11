package application

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
)

func concatBinaryResult(left typ.Type, right typ.Type) (typ.Type, bool) {
	if result, ok := concatPrimitiveResult(left, right); ok {
		return result, true
	}
	law, ok := Binary(kind.BinaryConcat)
	if !ok {
		return nil, false
	}
	return binaryMetamethodReturn(left, law.Meta, right)
}

func concatPrimitiveResult(left, right typ.Type) (typ.Type, bool) {
	if isConcatOperand(left) && isConcatOperand(right) {
		return typ.String, true
	}
	return nil, false
}
