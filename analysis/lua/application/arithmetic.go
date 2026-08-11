package application

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
)

func arithmeticBinaryResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
	if result, ok := arithmeticPrimitiveResult(left, op, right); ok {
		return result, true
	}
	if law, ok := Binary(op); ok && law.Meta != MetaAbsent {
		return binaryMetamethodReturn(left, law.Meta, right)
	}
	return nil, false
}

func arithmeticPrimitiveResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
	switch op {
	case kind.BinaryAdd, kind.BinarySub, kind.BinaryMul, kind.BinaryMod:
		if isIntegerish(left) && isIntegerish(right) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case kind.BinaryDiv, kind.BinaryPow:
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case kind.BinaryIDiv:
		if isIntegerish(left) && isIntegerish(right) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case kind.BinaryBitAnd, kind.BinaryBitOr, kind.BinaryBitXor, kind.BinaryShiftLeft, kind.BinaryShiftRight:
		if isIntegerConvertible(left) && isIntegerConvertible(right) {
			return typ.Integer, true
		}
	}
	return nil, false
}
