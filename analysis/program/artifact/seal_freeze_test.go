package artifact

import "testing"

func TestSealFreezeRejectsUnsealedArtifactIdentity(t *testing.T) {
	artifact := &Artifact{}
	if artifact.validUnsealed() {
		t.Fatal("unsealed artifact did not fail at seal")
	}
}
