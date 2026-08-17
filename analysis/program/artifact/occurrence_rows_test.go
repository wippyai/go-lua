package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestComputationOccurrenceRowsAreClosedAndPayloadScoped(t *testing.T) {
	id := identity.ContentID{1}
	operand := identity.ContentID{2}
	for _, kind := range []OccurrenceKind{OccurrenceUnary, OccurrenceSelect, OccurrenceValueClaim, OccurrenceReturnBoundary} {
		row := OccurrenceRow{kind: kind, id: id, inputs: []identity.ContentID{operand}}
		if !row.Available() || row.Kind() != kind || row.ID() != id {
			t.Fatalf("computation row kind %d was not a closed occurrence", kind)
		}
	}
	binaryCode, codeOK := binaryEqualityCode(flowkind.BinaryNotEqual, false, true)
	binary := OccurrenceRow{kind: OccurrenceBinaryEquality, id: id, inputs: []identity.ContentID{operand, {3}}, code: binaryCode}
	left, right, op, binaryOK := binary.BinaryEquality()
	if !codeOK || !binaryOK || left != operand || right != (identity.ContentID{3}) || op != flowkind.BinaryNotEqual {
		t.Fatal("binary equality row did not preserve ordered operands and polarity")
	}
	order := OccurrenceRow{kind: OccurrenceBinaryOrder, id: id, inputs: []identity.ContentID{operand, {4}}, code: uint64(flowkind.BinaryGreater)}
	left, right, op, orderOK := order.BinaryOrder()
	if !orderOK || left != operand || right != (identity.ContentID{4}) || op != flowkind.BinaryGreater {
		t.Fatal("binary order row did not preserve ordered operands and operator")
	}
	for _, hostile := range []OccurrenceRow{
		{kind: OccurrenceBinaryOrder, id: id, inputs: []identity.ContentID{operand, {4}}, code: uint64(flowkind.BinaryEqual)},
		{kind: OccurrenceBinaryOrder, id: id, inputs: []identity.ContentID{operand}, code: uint64(flowkind.BinaryLess)},
		{kind: OccurrenceBinaryOrder, id: id, inputs: []identity.ContentID{operand, {4}, {5}}, code: uint64(flowkind.BinaryLess)},
	} {
		if hostile.Available() {
			t.Fatal("binary order row accepted malformed operator/arity")
		}
	}
	literal := OccurrenceRow{kind: OccurrenceValueSource, id: id, inputs: []identity.ContentID{operand}, literalFamily: keyspace.FamilyString, literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "x"}, literalOK: true}
	if family, value, ok := literal.Literal(); !ok || family != keyspace.FamilyString || value.String != "x" {
		t.Fatal("literal payload did not remain scoped to ValueSource")
	}
	row := OccurrenceRow{kind: OccurrenceUnary, id: id, literalFamily: keyspace.FamilyString, literalOK: true}
	if row.Available() {
		t.Fatal("computation row accepted literal payload")
	}
}
