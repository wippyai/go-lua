package parserproducts

import "testing"

func TestGeneratedParserProductControlLawsNameOwnedProductions(t *testing.T) {
	rows := generatedControlProductLaws()
	if len(rows) == 0 {
		t.Fatal("generated control product-law denominator is empty")
	}
	for _, row := range rows {
		if row.Production == "" || row.Nonterminal == "" || row.ActionDigest == "" {
			t.Fatalf("incomplete generated control law %#v", row)
		}
	}
}
