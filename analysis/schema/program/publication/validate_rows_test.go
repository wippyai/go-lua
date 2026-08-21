package publication

import "testing"

func TestSealRowsRejectsNilValidationState(t *testing.T) {
	validator := &validator{}
	if validator.validateSealRows(nil) {
		t.Fatal("seal row phase admitted missing state")
	}
}
