package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestComputationOccurrenceRowsAreClosedAndPayloadScoped(t *testing.T) {
	id := identity.ContentID{1}
	operand := identity.ContentID{2}
	third := identity.ContentID{3}
	fourth := identity.ContentID{4}
	inputOperand, inputOperandOK := programschema.NewOccurrenceInput(operand)
	inputThird, inputThirdOK := programschema.NewOccurrenceInput(third)
	inputFourth, inputFourthOK := programschema.NewOccurrenceInput(fourth)
	if !inputOperandOK || !inputThirdOK || !inputFourthOK {
		t.Fatal("failed to construct canonical occurrence input fixture")
	}
	inputs := []programschema.OccurrenceInput{inputOperand, inputThird, inputOperand, inputFourth}
	for _, kind := range []programschema.OccurrenceKind{programschema.OccurrenceUnary, programschema.OccurrenceSelect, programschema.OccurrenceValueClaim, programschema.OccurrenceReturnBoundary} {
		row, rowOK := programschema.NewOccurrence(kind, id, identity.ContentID{}, 0, 0, 0, 0, 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
		if !rowOK || !row.Available() || row.Kind() != kind || row.ID() != id {
			t.Fatalf("computation row kind %d was not a closed occurrence", kind)
		}
	}
	binaryCode, codeOK := binaryEqualityCode(flowkind.BinaryNotEqual, false, true)
	binary, binaryRowOK := programschema.NewOccurrence(programschema.OccurrenceBinaryEquality, id, identity.ContentID{}, binaryCode, 0, 0, 0, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	binaryOffset, binaryCount, binarySpanOK := binary.InputSpan()
	left, right := inputs[int(binaryOffset)].InputID(), inputs[int(binaryOffset+1)].InputID()
	if !codeOK || !binaryRowOK || !occurrenceSemanticAvailable(binary) || !binarySpanOK || binaryCount != 2 || left != operand || right != third || flowkind.BinaryOp(binary.Code()&binaryEqualityCodeOpMask) != flowkind.BinaryNotEqual {
		t.Fatal("binary equality row did not preserve ordered operands and polarity")
	}
	order, orderRowOK := programschema.NewOccurrence(programschema.OccurrenceBinaryOrder, id, identity.ContentID{}, uint64(flowkind.BinaryGreater), 0, 0, 2, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	orderOffset, orderCount, orderSpanOK := order.InputSpan()
	left, right = inputs[int(orderOffset)].InputID(), inputs[int(orderOffset+1)].InputID()
	if !orderRowOK || !occurrenceSemanticAvailable(order) || !orderSpanOK || orderCount != 2 || left != operand || right != fourth || flowkind.BinaryOp(order.Code()) != flowkind.BinaryGreater {
		t.Fatal("binary order row did not preserve ordered operands and operator")
	}
	for _, hostile := range []struct {
		code  uint64
		count uint32
	}{
		{code: uint64(flowkind.BinaryEqual), count: 2},
		{code: uint64(flowkind.BinaryLess), count: 1},
		{code: uint64(flowkind.BinaryLess), count: 3},
	} {
		row, rowOK := programschema.NewOccurrence(programschema.OccurrenceBinaryOrder, id, identity.ContentID{}, hostile.code, 0, 0, 0, hostile.count, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
		if hostile.count != 2 {
			if rowOK || row.Available() {
				t.Fatal("binary order row accepted malformed arity")
			}
			continue
		}
		if occurrenceSemanticAvailable(row) {
			t.Fatal("binary order row accepted malformed operator/arity")
		}
	}
	literal, literalRowOK := programschema.NewOccurrence(programschema.OccurrenceValueSource, id, identity.ContentID{}, 0, 0, 0, 0, 1, keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "x"}, true)
	family, value, literalOK := literal.Literal()
	if !literalRowOK || !literalOK || family != keyspace.FamilyString || value.String != "x" {
		t.Fatal("literal payload did not remain scoped to ValueSource")
	}
	row, rowOK := programschema.NewOccurrence(programschema.OccurrenceUnary, id, identity.ContentID{}, 0, 0, 0, 0, 1, keyspace.FamilyString, keyspace.LiteralValue{}, true)
	if rowOK || row.Available() {
		t.Fatal("computation row accepted literal payload")
	}
}
