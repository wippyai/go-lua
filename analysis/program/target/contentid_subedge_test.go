package target

import "testing"

func TestContentIDSubedgeEncodingTracksRouteAuthority(t *testing.T) {
	leftSpec := Spec{Operations: []OperationSpec{protectedSubedgeOperation("subedge-id", false, false, false)}}
	rightSpec := Spec{Operations: []OperationSpec{protectedSubedgeOperation("subedge-id", true, false, false)}}
	left := mustSeal(t, leftSpec).ContentID()
	right := mustSeal(t, rightSpec).ContentID()
	if left == right {
		t.Fatal("subedge route mutation was omitted from ContentID")
	}
}
