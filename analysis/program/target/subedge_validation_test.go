package target

import "testing"

func TestSubedgeValidationRejectsInvalidFamilyAuthority(t *testing.T) {
	spec := Spec{Operations: []OperationSpec{protectedSubedgeOperation("invalid-edge", false, false, false)}}
	spec.Operations[0].Subedges[0].Family = SubedgeFamilyInvalid
	if _, err := testSeal(&spec); err == nil {
		t.Fatal("invalid subedge family was accepted")
	}
}
