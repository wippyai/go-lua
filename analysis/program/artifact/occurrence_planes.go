package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// occurrenceLookup and occurrenceSpanGeometry are compiler-only geometry. The
// lookup keys the authored span proof while it is live; neither type crosses
// the publication boundary or becomes a Program index.
type occurrenceLookup struct {
	kind programschema.OccurrenceKind
	id   identity.ContentID
}

type occurrenceSpanGeometry struct {
	entry  []identity.ContentID
	finish []identity.ContentID
	route  identity.ContentID
}

const (
	binaryEqualityCodeOpMask        = uint64(0xff)
	binaryEqualityCodeHasComparison = uint64(1 << 8)
	binaryEqualityCodeInvert        = uint64(1 << 9)

	operationPredicateCodeOpMask = uint64(0xff)
	operationPredicateCodeTruth  = uint64(1 << 8)
)

func binaryEqualityCode(op flowkind.BinaryOp, hasComparison, invert bool) (uint64, bool) {
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

// occurrenceDenseAvailable checks a canonical parent row while its child
// spans are still resident in compiler-owned planes. Parent semantics live on
// the canonical row; child identity validity and span bounds are established
// against the dense planes while the compiler still owns them.
func occurrenceDenseAvailable(row programschema.Occurrence, points []programschema.OccurrencePoint, inputs []programschema.OccurrenceInput) bool {
	if !occurrenceSemanticAvailable(row) {
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

// occurrenceSemanticAvailable keeps the closed opcode meanings in the
// compiler domain. The canonical schema validates only row shape and spans;
// this check authenticates the artifact's flow-owned code vocabulary before a
// row is consumed or sealed.
func occurrenceSemanticAvailable(row programschema.Occurrence) bool {
	if !row.Available() {
		return false
	}
	_, inputCount, inputOK := row.InputSpan()
	if !inputOK {
		return false
	}
	switch row.Kind() {
	case programschema.OccurrenceBinaryEquality:
		code := row.Code()
		op := flowkind.BinaryOp(code & binaryEqualityCodeOpMask)
		hasComparison := code&binaryEqualityCodeHasComparison != 0
		invert := code&binaryEqualityCodeInvert != 0
		return code&^(binaryEqualityCodeOpMask|binaryEqualityCodeHasComparison|binaryEqualityCodeInvert) == 0 &&
			(op == flowkind.BinaryEqual || op == flowkind.BinaryNotEqual) && invert == (op == flowkind.BinaryNotEqual) &&
			((!hasComparison && inputCount == 2) || (hasComparison && inputCount == 5))
	case programschema.OccurrenceBinaryArithmetic:
		return inputCount == 2 && flowkind.IsBinaryArithmetic(flowkind.BinaryOp(row.Code()))
	case programschema.OccurrenceBinaryOrder:
		return inputCount == 2 && flowkind.IsBinaryOrder(flowkind.BinaryOp(row.Code()))
	case programschema.OccurrenceBinaryPresenceRefinement:
		return row.Code() <= 1
	case programschema.OccurrenceOperationPredicateRefinement:
		code := row.Code()
		op := flowkind.BinaryOp(code & operationPredicateCodeOpMask)
		return code&^(operationPredicateCodeOpMask|operationPredicateCodeTruth) == 0 &&
			(op == flowkind.BinaryEqual || op == flowkind.BinaryNotEqual)
	default:
		return true
	}
}

func occurrencePointID(row programschema.Occurrence, points []programschema.OccurrencePoint, index int) (identity.ContentID, bool) {
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

func occurrencePointIDs(row programschema.Occurrence, points []programschema.OccurrencePoint) ([]identity.ContentID, bool) {
	offset, count, spanOK := row.PointSpan()
	if !spanOK || uint64(offset)+uint64(count) > uint64(len(points)) {
		return nil, false
	}
	result := make([]identity.ContentID, 0, count)
	for index := uint32(0); index < count; index++ {
		point, pointOK := occurrencePointID(row, points, int(index))
		if !pointOK {
			return nil, false
		}
		result = append(result, point)
	}
	return result, true
}

func occurrenceInputID(row programschema.Occurrence, inputs []programschema.OccurrenceInput, index int) (identity.ContentID, bool) {
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

func occurrenceInputCount(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (int, bool) {
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

func occurrenceValueSourceSpanID(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (identity.ContentID, bool) {
	if row.Kind() != programschema.OccurrenceValueSource {
		return identity.ContentID{}, false
	}
	return occurrenceInputID(row, inputs, 0)
}

func occurrenceStorageRead(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (cell, span identity.ContentID, ok bool) {
	if row.Kind() != programschema.OccurrenceStorageRead {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	cell, cellOK := occurrenceInputID(row, inputs, 0)
	span, spanOK := occurrenceInputID(row, inputs, 1)
	return cell, span, cellOK && spanOK
}

func occurrenceBinaryEquality(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != programschema.OccurrenceBinaryEquality {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := occurrenceInputID(row, inputs, 0)
	right, rightOK := occurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code() & binaryEqualityCodeOpMask), leftOK && rightOK
}

func occurrenceBinaryArithmetic(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != programschema.OccurrenceBinaryArithmetic {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := occurrenceInputID(row, inputs, 0)
	right, rightOK := occurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code()), leftOK && rightOK
}

func occurrenceBinaryOrder(row programschema.Occurrence, inputs []programschema.OccurrenceInput) (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if row.Kind() != programschema.OccurrenceBinaryOrder {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	left, leftOK := occurrenceInputID(row, inputs, 0)
	right, rightOK := occurrenceInputID(row, inputs, 1)
	return left, right, flowkind.BinaryOp(row.Code()), leftOK && rightOK
}
