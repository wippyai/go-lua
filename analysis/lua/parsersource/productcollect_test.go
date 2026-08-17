package parsersource

import "testing"

func TestProductCollectionFindsParserHelpersAndProductions(t *testing.T) {
	analysis, err := DiscoverProducts(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Products) == 0 || len(analysis.Uses) == 0 {
		t.Fatalf("product collection = products %d uses %d, want both non-empty", len(analysis.Products), len(analysis.Uses))
	}
	for _, product := range analysis.Products {
		if product.Owner == "" || product.Constructor == "" || product.Scope == ProductScopeInvalid {
			t.Fatalf("incomplete collected product %#v", product)
		}
	}
}
