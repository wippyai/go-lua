package artifact

import (
	"testing"

	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticInputRowsRequireTheirDeclaredOperandShape(t *testing.T) {
	row := StaticInputRow{id: valuesLawID(1), owner: valuesLawID(2), kind: StaticInputTypeOf, expression: valuesLawID(3), source: valuesLawID(4), target: valuesLawID(5), operand: valuesLawID(6), frontier: valuesLawID(7), operandKind: programstatic.StaticOperandKnown}
	if !row.Available() || row.ExpressionID() != valuesLawID(3) {
		t.Fatal("valid static input row unavailable")
	}
	row.expression = valuesLawID(0)
	if row.Available() {
		t.Fatal("static input row admitted missing expression")
	}
}
