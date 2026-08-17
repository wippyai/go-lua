package grammarproof

import "testing"

func TestGeneratedASTCodecContractRemainsCurrent(t *testing.T) {
	if err := ValidateGeneratedASTCodec(moduleRoot(t)); err != nil {
		t.Fatal(err)
	}
}
