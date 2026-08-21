package publication

import "testing"

func TestSealFreezeRejectsUnsealedArtifactIdentity(t *testing.T) {
	validator := &validator{}
	if validator.validate() {
		t.Fatal("unsealed artifact did not fail at seal")
	}
}
