package candidates

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// classifyUnary is the sole Unary candidate disposition. Every current enum
// member is named explicitly; UnaryNot is intentionally a valid no-bucket
// operation, while an unknown value is rejected rather than silently erased.
func classifyUnary(op kind.UnaryOp) (uint8, error) {
	switch op {
	case kind.UnaryNeg:
		return unaryNumericCandidate, nil
	case kind.UnaryNot:
		return unaryNoCandidate, nil
	case kind.UnaryLen:
		return unaryLengthCandidate, nil
	case kind.UnaryBitNot:
		return unaryNumericCandidate, nil
	default:
		return unaryNoCandidate, errors.New("program/flow/candidates: invalid UnaryOp")
	}
}

// classifyBinary is the sole Binary candidate disposition. The switch lists
// all current operators explicitly so a future enum value cannot fall into a
// bucket by accident.
func classifyBinary(op kind.BinaryOp) (uint8, error) {
	switch op {
	case kind.BinaryAdd:
		return binaryArithmeticCandidate, nil
	case kind.BinarySub:
		return binaryArithmeticCandidate, nil
	case kind.BinaryMul:
		return binaryArithmeticCandidate, nil
	case kind.BinaryDiv:
		return binaryArithmeticCandidate, nil
	case kind.BinaryIDiv:
		return binaryArithmeticCandidate, nil
	case kind.BinaryMod:
		return binaryArithmeticCandidate, nil
	case kind.BinaryPow:
		return binaryArithmeticCandidate, nil
	case kind.BinaryConcat:
		return binaryConcatCandidate, nil
	case kind.BinaryBitAnd:
		return binaryBitwiseCandidate, nil
	case kind.BinaryBitOr:
		return binaryBitwiseCandidate, nil
	case kind.BinaryBitXor:
		return binaryBitwiseCandidate, nil
	case kind.BinaryShiftLeft:
		return binaryBitwiseCandidate, nil
	case kind.BinaryShiftRight:
		return binaryBitwiseCandidate, nil
	case kind.BinaryEqual:
		return binaryEqualityCandidate, nil
	case kind.BinaryNotEqual:
		return binaryEqualityCandidate, nil
	case kind.BinaryLess:
		return binaryOrderCandidate, nil
	case kind.BinaryLessEqual:
		return binaryOrderCandidate, nil
	case kind.BinaryGreater:
		return binaryOrderCandidate, nil
	case kind.BinaryGreaterEqual:
		return binaryOrderCandidate, nil
	default:
		return binaryNoCandidate, errors.New("program/flow/candidates: invalid BinaryOp")
	}
}

// classifySelect proves the complete Select vocabulary while intentionally
// producing no candidate disposition for either short-circuit operator.
func classifySelect(op kind.SelectOp) error {
	switch op {
	case kind.SelectAnd:
		return nil
	case kind.SelectOr:
		return nil
	default:
		return errors.New("program/flow/candidates: invalid SelectOp")
	}
}
