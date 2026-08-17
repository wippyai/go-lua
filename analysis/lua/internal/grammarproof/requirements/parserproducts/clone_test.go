package parserproducts

import "testing"

func TestParserProductsCloneDetachesNestedEvidenceRows(t *testing.T) {
	copy := clone(Generated)
	copy.Fields[0].Form = "changed"
	copy.ProductLaws[0].RHS[0] = "changed"
	if Generated.Fields[0].Form == "changed" || Generated.ProductLaws[0].RHS[0] == "changed" {
		t.Fatal("parser-products clone shares nested evidence storage")
	}
}
