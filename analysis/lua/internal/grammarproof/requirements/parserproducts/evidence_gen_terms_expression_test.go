package parserproducts

import "testing"

func TestGeneratedParserProductExpressionLawsNameConstructedFields(t *testing.T) {
	rows := generatedExpressionProductLaws()
	if len(rows) == 0 {
		t.Fatal("generated expression product-law denominator is empty")
	}
	var constructed bool
	for _, row := range rows {
		if row.Production == "" || row.ActionDigest == "" {
			t.Fatalf("incomplete generated expression law %#v", row)
		}
		constructed = constructed || len(row.Products) != 0
	}
	if !constructed {
		t.Fatal("generated expression laws contain no constructor products")
	}
}
