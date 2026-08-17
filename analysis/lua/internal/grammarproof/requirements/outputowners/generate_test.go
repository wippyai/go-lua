package outputowners

import (
	"bytes"
	"testing"
)

func TestOutputOwnerGeneratorRendersDeterministicGeneratedContract(t *testing.T) {
	first, err := render(Generated)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(Generated)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("output-owner generator is not deterministic")
	}
	if !bytes.Contains(first, []byte("Code generated")) || !bytes.Contains(first, []byte("Generated")) {
		t.Fatal("rendered output-owner contract lacks generated evidence declaration")
	}
}
