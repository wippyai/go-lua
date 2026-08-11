package equation

import "testing"

func TestVariantPlanPrototypeRowReplayLaw(t *testing.T) {
	leftData := &variantPlanData{key: boundaryKey(241)}
	rightData := &variantPlanData{key: boundaryKey(242)}
	row := PrototypeRow{
		plan:   leftData,
		key:    boundaryKey(243),
		schema: boundaryKey(244),
		family: boundaryKey(245),
	}
	// Availability also authenticates occurrence/operand, so use an issued
	// row from one small sealed batch rather than a fabricated coordinate.
	_, occurrence, operands := operandClosureFixture(t, 1)
	row.occurrence, row.operand = occurrence, operands[0]
	left := VariantPlan{data: leftData}
	right := VariantPlan{data: rightData}
	if !left.OwnsPrototypeRow(row) {
		t.Fatal("issuing plan rejected its own prototype row")
	}
	if right.OwnsPrototypeRow(row) {
		t.Fatal("prototype row replayed across distinct plan identity")
	}
}
