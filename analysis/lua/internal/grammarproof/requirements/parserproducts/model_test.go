package parserproducts

import "testing"

func TestParserProductsModelExposesActionArenaAndEvidenceCounts(t *testing.T) {
	if Generated.FieldCount() != len(Generated.Fields) || Generated.ProductCount() != len(Generated.Products) {
		t.Fatal("evidence count accessors disagree with their owned rows")
	}
	terms := Generated.ActionTerms
	if len(terms.Symbols) == 0 || len(terms.Scopes) == 0 || len(terms.Terms) == 0 {
		t.Fatalf("action arena = symbols %d scopes %d terms %d, want all populated", len(terms.Symbols), len(terms.Scopes), len(terms.Terms))
	}
	if _, ok := terms.Term(ActionTermID(1)); !ok {
		t.Fatal("action arena failed to resolve its first term")
	}
}
