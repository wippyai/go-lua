package targetingress

import (
	"bytes"
	"testing"
)

func TestTargetIngressGeneratorRendersDeterministicSemanticArtifact(t *testing.T) {
	evidence, err := Current()
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
		t.Fatal("target ingress generator output is not deterministic")
	}
}
