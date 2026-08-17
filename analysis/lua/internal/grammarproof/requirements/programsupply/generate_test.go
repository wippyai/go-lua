package programsupply

import (
	"bytes"
	"testing"
)

func TestProgramSupplyGeneratorRendersDeterministicTypedArtifact(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	first, err := render(evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !bytes.Equal(first, second) || !bytes.Contains(first, []byte("Code generated")) {
		t.Fatal("Program supply generator is not deterministic")
	}
}
