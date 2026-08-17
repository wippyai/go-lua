package parsersource

import (
	"fmt"
	"testing"
)

func TestProbeDual(t *testing.T) {
	root := moduleRoot(t)
	analysis, err := DiscoverProducts(root)
	if err != nil {
		t.Fatal(err)
	}
	consumed := make(map[string]bool)
	for _, use := range analysis.Uses {
		for _, source := range use.Sources {
			consumed[fmt.Sprintf("%s#%d", use.Owner, source)] = true
		}
	}
	roots, orphans := 0, 0
	for _, product := range analysis.Products {
		if product.Root {
			roots++
			continue
		}
		key := fmt.Sprintf("%s#%d", product.Owner, product.Ordinal)
		if !consumed[key] {
			orphans++
			fmt.Println("unconsumed non-root:", key, product.Constructor)
		}
	}
	fmt.Println("products:", len(analysis.Products), "roots:", roots, "unconsumed non-root:", orphans)
}
