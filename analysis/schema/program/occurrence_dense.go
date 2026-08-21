package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

const (
	binaryEqualityCodeOpMask        = uint64(0xff)
	binaryEqualityCodeHasComparison = uint64(1 << 8)
	binaryEqualityCodeInvert        = uint64(1 << 9)

	operationPredicateCodeOpMask = uint64(0xff)
	operationPredicateCodeTruth  = uint64(1 << 8)
)

// OccurrenceBinaryEqualityCode encodes the closed binary-equality operand
// shape carried by a canonical Occurrence row.
func OccurrenceBinaryEqualityCode(op flowkind.BinaryOp, hasComparison, invert bool) (uint64, bool) {
	if (op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) || invert != (op == flowkind.BinaryNotEqual) {
		return 0, false
	}
	code := uint64(op)
	if hasComparison {
		code |= binaryEqualityCodeHasComparison
	}
	if invert {
		code |= binaryEqualityCodeInvert
	}
	return code, true
}

// OccurrenceOperationPredicateCode encodes the closed guarded operation
// predicate carried by an OccurrenceOperationPredicateRefinement row. The
// operation is the canonical Flow equality opcode; truth is the branch
// outcome, represented by the schema-owned high bit. Callers must not encode
// this shape locally: the schema owns both the vocabulary and its wire code.
func OccurrenceOperationPredicateCode(op flowkind.BinaryOp, truth bool) (uint64, bool) {
	if op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual {
		return 0, false
	}
	code := uint64(op)
	if truth {
		code |= operationPredicateCodeTruth
	}
	return code, true
}

// OccurrenceDenseAvailable validates one canonical parent row against its
// dense point and input child planes. The child slices are construction-time
// canonical rows, not compiler-owned draft vocabulary.
func OccurrenceDenseAvailable(row Occurrence, points []OccurrencePoint, inputs []OccurrenceInput) bool {
	if !OccurrenceSemanticAvailable(row) {
		return false
	}
	pointOffset, pointCount, pointOK := row.PointSpan()
	inputOffset, inputCount, inputOK := row.InputSpan()
	if !pointOK || !inputOK || uint64(pointOffset)+uint64(pointCount) > uint64(len(points)) || uint64(inputOffset)+uint64(inputCount) > uint64(len(inputs)) {
		return false
	}
	for index := uint32(0); index < pointCount; index++ {
		if !points[int(pointOffset+index)].Available() {
			return false
		}
	}
	for index := uint32(0); index < inputCount; index++ {
		if !inputs[int(inputOffset+index)].Available() {
			return false
		}
	}
	return true
}

// OccurrenceSemanticAvailable validates the closed flow opcode vocabulary
// encoded by a canonical occurrence row.
func OccurrenceSemanticAvailable(row Occurrence) bool {
	if !row.Available() {
		return false
	}
	_, inputCount, inputOK := row.InputSpan()
	if !inputOK {
		return false
	}
	switch row.Kind() {
	case OccurrenceBinaryEquality:
		code := row.Code()
		op := flowkind.BinaryOp(code & binaryEqualityCodeOpMask)
		hasComparison := code&binaryEqualityCodeHasComparison != 0
		invert := code&binaryEqualityCodeInvert != 0
		return code&^(binaryEqualityCodeOpMask|binaryEqualityCodeHasComparison|binaryEqualityCodeInvert) == 0 &&
			(op == flowkind.BinaryEqual || op == flowkind.BinaryNotEqual) && invert == (op == flowkind.BinaryNotEqual) &&
			((!hasComparison && inputCount == 2) || (hasComparison && inputCount == 5))
	case OccurrenceBinaryArithmetic:
		return inputCount == 2 && flowkind.IsBinaryArithmetic(flowkind.BinaryOp(row.Code()))
	case OccurrenceBinaryOrder:
		return inputCount == 2 && flowkind.IsBinaryOrder(flowkind.BinaryOp(row.Code()))
	case OccurrenceBinaryPresenceRefinement:
		return row.Code() <= 1
	case OccurrenceOperationPredicateRefinement:
		code := row.Code()
		op := flowkind.BinaryOp(code & operationPredicateCodeOpMask)
		return code&^(operationPredicateCodeOpMask|operationPredicateCodeTruth) == 0 &&
			(op == flowkind.BinaryEqual || op == flowkind.BinaryNotEqual)
	default:
		return true
	}
}

// OccurrencePointID resolves one point child at its canonical parent-relative
// position.
func OccurrencePointID(row Occurrence, points []OccurrencePoint, index int) (identity.ContentID, bool) {
	if index < 0 || !row.Available() {
		return identity.ContentID{}, false
	}
	offset, count, spanOK := row.PointSpan()
	if !spanOK || uint64(index) >= uint64(count) || uint64(offset)+uint64(index) >= uint64(len(points)) {
		return identity.ContentID{}, false
	}
	point := points[int(offset)+index]
	if !point.Available() {
		return identity.ContentID{}, false
	}
	return point.PointID(), true
}

// OccurrencePointIDs resolves the complete ordered point child span.
func OccurrencePointIDs(row Occurrence, points []OccurrencePoint) ([]identity.ContentID, bool) {
	offset, count, spanOK := row.PointSpan()
	if !spanOK || uint64(offset)+uint64(count) > uint64(len(points)) {
		return nil, false
	}
	result := make([]identity.ContentID, 0, count)
	for index := uint32(0); index < count; index++ {
		point, pointOK := OccurrencePointID(row, points, int(index))
		if !pointOK {
			return nil, false
		}
		result = append(result, point)
	}
	return result, true
}

// OccurrenceInputID resolves one input child at its canonical parent-relative
// position.
func OccurrenceInputID(row Occurrence, inputs []OccurrenceInput, index int) (identity.ContentID, bool) {
	if index < 0 || !row.Available() {
		return identity.ContentID{}, false
	}
	offset, count, spanOK := row.InputSpan()
	if !spanOK || uint64(index) >= uint64(count) || uint64(offset)+uint64(index) >= uint64(len(inputs)) {
		return identity.ContentID{}, false
	}
	input := inputs[int(offset)+index]
	if !input.Available() {
		return identity.ContentID{}, false
	}
	return input.InputID(), true
}

// OccurrenceInputCount validates and returns the complete input width.
func OccurrenceInputCount(row Occurrence, inputs []OccurrenceInput) (int, bool) {
	offset, count, spanOK := row.InputSpan()
	if !spanOK || uint64(offset)+uint64(count) > uint64(len(inputs)) {
		return 0, false
	}
	for index := uint32(0); index < count; index++ {
		if !inputs[int(offset+index)].Available() {
			return 0, false
		}
	}
	return int(count), true
}

// OccurrenceValueSourceSpanID resolves the sole semantic input of a value
// source occurrence.
func OccurrenceValueSourceSpanID(row Occurrence, inputs []OccurrenceInput) (identity.ContentID, bool) {
	if row.Kind() != OccurrenceValueSource {
		return identity.ContentID{}, false
	}
	return OccurrenceInputID(row, inputs, 0)
}

// OccurrenceStorageReadOperands resolves the cell and source-span operands of a
// storage-read occurrence.
func OccurrenceStorageReadOperands(row Occurrence, inputs []OccurrenceInput) (cell, span identity.ContentID, ok bool) {
	if row.Kind() != OccurrenceStorageRead {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	cell, cellOK := OccurrenceInputID(row, inputs, 0)
	span, spanOK := OccurrenceInputID(row, inputs, 1)
	return cell, span, cellOK && spanOK
}

// OccurrenceBinaryEqualityOperands resolves the ordered equality operands and
// canonical closed opcode.
func OccurrenceBinaryEqualityOperands(row Occurrence, inputs []OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != OccurrenceBinaryEquality {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := OccurrenceInputID(row, inputs, 0)
	right, rightOK := OccurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code() & binaryEqualityCodeOpMask), leftOK && rightOK
}

// OccurrenceBinaryArithmeticOperands resolves the ordered arithmetic operands and
// canonical closed opcode.
func OccurrenceBinaryArithmeticOperands(row Occurrence, inputs []OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != OccurrenceBinaryArithmetic {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := OccurrenceInputID(row, inputs, 0)
	right, rightOK := OccurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code()), leftOK && rightOK
}

// OccurrenceBinaryOrderOperands resolves the ordered comparison operands and
// canonical closed opcode.
func OccurrenceBinaryOrderOperands(row Occurrence, inputs []OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != OccurrenceBinaryOrder {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := OccurrenceInputID(row, inputs, 0)
	right, rightOK := OccurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code()), leftOK && rightOK
}
