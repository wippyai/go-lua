package causal

import "testing"

func TestClosedArmPredicatesRejectZeroAndUnknown(t *testing.T) {
	for _, arm := range []BoundaryArmKind{0, BoundaryCancel + 1, ^BoundaryArmKind(0)} {
		successor := Successor{Arm: arm}
		if successor.IsLocal() || successor.IsBoundary() {
			t.Fatalf("malformed arm %d was classified as a semantic plane", arm)
		}
		if _, _, _, ok := boundarySuccessor(CallBoundary{}, arm); ok {
			t.Fatalf("malformed arm %d produced a boundary route", arm)
		}
	}
}
