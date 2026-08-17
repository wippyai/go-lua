package control

import "testing"

func TestLabelAuthorityRejectsAbsentPredeclaration(t *testing.T) {
	var writer Writer
	if _, err := writer.Label(nil); err == nil {
		t.Fatal("Label accepted an absent predeclaration")
	}
}
