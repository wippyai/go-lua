package artifact

import "testing"

func TestSealRowsRejectsNilValidationState(t *testing.T) {
	artifact := &Artifact{}
	if failure := artifact.validateSealRows(nil); !failure.Available() || failure.Stage() != CompileStageSeal {
		t.Fatal("seal row phase admitted missing state")
	}
}
