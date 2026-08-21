package publication

import "testing"

func TestSealIndexRejectsNilValidationState(t *testing.T) {
	validator := &validator{}
	if validator.validateSealIndexes(nil) {
		t.Fatal("seal index phase admitted missing state")
	}
}
