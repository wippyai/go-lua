package parserproducts

import "testing"

func TestGeneratedParserProductStateProductsNameObservedConstructors(t *testing.T) {
	rows := generatedProducts()
	if len(rows) == 0 || len(rows) != len(Generated.Products) {
		t.Fatalf("generated products = %d/%d", len(rows), len(Generated.Products))
	}
	var states bool
	for _, row := range rows {
		if row.Form == "" || row.Source == "" {
			t.Fatalf("incomplete generated product %#v", row)
		}
		states = states || len(row.States) != 0
	}
	if !states {
		t.Fatal("generated products contain no state-bearing constructor")
	}
}
