package parserproducts

import (
	"bytes"
	"testing"
)

func TestParserProductsGeneratorRendersEveryGeneratedContractDeterministically(t *testing.T) {
	first, err := renderFiles(Generated)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderFiles(Generated)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range expectedGeneratedFiles() {
		left, leftOK := first[name]
		right, rightOK := second[name]
		if !leftOK || !rightOK || len(left) == 0 || !bytes.Equal(left, right) {
			t.Fatalf("generated parser-products file %s is missing or nondeterministic", name)
		}
	}
}
