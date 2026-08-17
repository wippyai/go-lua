package parsersource

import "testing"

func TestProductResultPublishesRootConstructionVectorsAndMutations(t *testing.T) {
	analysis, err := DiscoverProducts(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	var roots, fields int
	for _, product := range analysis.Products {
		if product.Root {
			roots++
		}
		fields += len(product.Fields)
	}
	if roots == 0 || fields == 0 || len(analysis.Mutations) == 0 {
		t.Fatalf("product result roots=%d fields=%d mutations=%d, want all semantic lanes", roots, fields, len(analysis.Mutations))
	}
	for _, mutation := range analysis.Mutations {
		if mutation.Owner == "" || mutation.Constructor == "" || mutation.Field == "" || len(mutation.States) == 0 {
			t.Fatalf("incomplete mutation row %#v", mutation)
		}
	}
}
