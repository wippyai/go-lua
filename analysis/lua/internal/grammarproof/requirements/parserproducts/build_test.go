package parserproducts

import "testing"

func TestParserProductsBuildArtifactHasSealedStructure(t *testing.T) {
	if err := Generated.validateStructural(); err != nil {
		t.Fatal(err)
	}
	if Generated.Digest == "" || Generated.FieldCount() == 0 || Generated.ProductCount() == 0 {
		t.Fatalf("generated parser-products artifact = fields %d products %d digest %q", Generated.FieldCount(), Generated.ProductCount(), Generated.Digest)
	}
}
