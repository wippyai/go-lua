package parseruses

import "testing"

func TestParserUsesAxesKeepContextFamiliesDistinct(t *testing.T) {
	cases := []struct {
		target ProgramUseClass
		want   ValuesPosition
	}{
		{ProgramUseExpression, ValuesPositionScalar},
		{ProgramUseValues, ValuesPositionFinalOpen},
		{ProgramUseControl, ValuesPositionNotApplicable},
	}
	for _, test := range cases {
		child := "Expr"
		if test.target == ProgramUseControl {
			child = "Stmt"
		}
		if got := valuesPosition(test.target, child); got != test.want {
			t.Fatalf("valuesPosition(%d) = %d, want %d", test.target, got, test.want)
		}
	}
	for _, child := range []string{"Expr", "Stmt", "TypeExpr", "ValuesAdjustment"} {
		if childClass(child) == ChildClassInvalid {
			t.Fatalf("childClass(%q) is invalid", child)
		}
	}
}
