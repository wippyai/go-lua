package parserproducts

import "testing"

func TestGeneratedParserProductHelperLawsRetainReturnAndApplicationRelations(t *testing.T) {
	rows := generatedHelperLaws()
	if len(rows) == 0 || len(rows) != len(Generated.HelperLaws) {
		t.Fatalf("generated helper laws = %d/%d", len(rows), len(Generated.HelperLaws))
	}
	var returns, applications bool
	for _, row := range rows {
		returns = returns || len(row.Returns) != 0
		applications = applications || len(row.Products) != 0 || len(row.Helpers) != 0
	}
	if !returns || !applications {
		t.Fatal("generated helper laws omit return or application evidence")
	}
}
