package artifact

import "testing"

func TestSealFreezeRejectsUnsealedArtifactIdentity(t *testing.T) {
	artifact := &Artifact{}
	failure := artifact.validUnsealedFailure()
	if !failure.Available() || failure.Stage() != CompileStageSeal {
		t.Fatal("unsealed artifact did not fail at seal")
	}
}
