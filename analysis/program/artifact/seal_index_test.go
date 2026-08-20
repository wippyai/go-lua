package artifact

import "testing"

func TestSealIndexRejectsNilValidationState(t *testing.T) {
	artifact := &Artifact{}
	if artifact.validateSealIndexes(nil) {
		t.Fatal("seal index phase admitted missing state")
	}
}
