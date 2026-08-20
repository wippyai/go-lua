package artifact

import "testing"

func TestSealRowsRejectsNilValidationState(t *testing.T) {
	artifact := &Artifact{}
	if artifact.validateSealRows(nil) {
		t.Fatal("seal row phase admitted missing state")
	}
}
