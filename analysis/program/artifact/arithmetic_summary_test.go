package artifact

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestArithmeticSummarySealCommitsGuardAndOperatorShape(t *testing.T) {
	occurrence := digest("analysis/program-artifact/test/arithmetic-occurrence", artifactFormat, uintField(1))
	body := digest("analysis/program-artifact/test/arithmetic-body", artifactFormat, uintField(1))
	row := ArithmeticSummaryRow{
		occurrence: occurrence,
		body:       body,
		op:         flowkind.BinaryIDiv,
		left:       NumericRepresentationInteger,
		right:      NumericRepresentationInteger,
		result:     NumericRepresentationInteger,
		divisor:    ArithmeticDivisorNonzeroNotMinusOne,
	}
	row.id = arithmeticSummaryID(row.occurrence, row.body, row.op, row.left, row.right, row.result, row.divisor)
	if !row.Available() {
		t.Fatal("valid guarded arithmetic summary unavailable")
	}
	mutated := row
	mutated.divisor = ArithmeticDivisorNonzero
	if mutated.Available() {
		t.Fatal("stored arithmetic summary accepted a divisor mutation")
	}
	mutated = row
	mutated.op = flowkind.BinaryAdd
	if mutated.Available() {
		t.Fatal("guarded divisor crossed to a non-division operator")
	}
	if id := arithmeticSummaryID(row.occurrence, row.body, flowkind.BinaryAdd, row.left, row.right, row.result, row.divisor); id.Available() {
		t.Fatal("arithmetic summary ID admitted a guarded non-division operator")
	}
	mutated = row
	mutated.id = arithmeticSummaryID(row.occurrence, row.body, row.op, row.left, row.right, row.result, ArithmeticDivisorNone)
	if mutated.Available() {
		t.Fatal("unguarded seal substituted for guarded row content")
	}
}

func TestUnarySummarySealCommitsExactOutputAndRepresentation(t *testing.T) {
	occurrence := digest("analysis/program-artifact/test/unary-occurrence", artifactFormat, uintField(1))
	body := digest("analysis/program-artifact/test/unary-body", artifactFormat, uintField(1))
	point := digest("analysis/program-artifact/test/unary-point", artifactFormat, uintField(1))
	row := UnarySummaryRow{
		occurrence: occurrence, body: body, point: point, op: flowkind.UnaryNeg,
		operand: NumericRepresentationInteger, result: NumericRepresentationInteger,
	}
	row.id = unarySummaryID(row.occurrence, row.body, row.point, row.op, row.operand, row.result)
	if !row.Available() {
		t.Fatal("valid unary summary unavailable")
	}
	mutated := row
	mutated.point = digest("analysis/program-artifact/test/unary-point", artifactFormat, uintField(2))
	if mutated.Available() {
		t.Fatal("unary summary accepted an output-point splice")
	}
	mutated = row
	mutated.result = NumericRepresentationFloat
	if mutated.Available() {
		t.Fatal("unary summary accepted a changed representation arm")
	}
	if id := unarySummaryID(row.occurrence, row.body, row.point, flowkind.UnaryNot, row.operand, row.result); id.Available() {
		t.Fatal("unary summary ID admitted a non-negation operator")
	}
}
