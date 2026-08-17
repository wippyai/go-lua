package artifact

import "testing"

func TestUnarySummaryRowsRejectUnknownRepresentation(t *testing.T) {
	row := UnarySummaryRow{occurrence: valuesLawID(1), body: valuesLawID(2), point: valuesLawID(3), op: 1, operand: NumericRepresentation(0), result: NumericRepresentationInteger}
	row.id = unarySummaryID(row.occurrence, row.body, row.point, row.op, row.operand, row.result)
	if row.Available() {
		t.Fatal("unary summary admitted an unknown operand representation")
	}
}
