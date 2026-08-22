package parseruses

import "testing"

func TestParserUsesBuildSealsDirectAndTypedRoutes(t *testing.T) {
	products := testProducts()
	inventory := testCensus(products)
	evidence, err := Build(products, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(products, inventory); err != nil {
		t.Fatal(err)
	}
	if len(evidence.UseSlots) == 0 || len(evidence.UsePaths) == 0 || evidence.ProductsDigest != products.Digest {
		t.Fatalf("parser-use evidence = %#v, want slots paths and source digest", evidence)
	}
}
