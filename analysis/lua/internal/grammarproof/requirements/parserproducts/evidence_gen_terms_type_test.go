package parserproducts

import "testing"

func TestGeneratedParserProductTypeLawsNameStaticRelations(t *testing.T) {
	rows := generatedTypeProductLaws()
	if len(rows) == 0 {
		t.Fatal("generated type product-law denominator is empty")
	}
	for _, row := range rows {
		if row.Production == "" || row.ActionDigest == "" {
			t.Fatalf("incomplete generated type law %#v", row)
		}
	}
}
