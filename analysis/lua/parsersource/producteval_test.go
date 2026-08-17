package parsersource

import "testing"

func TestProductValueJoinPreservesBothStatesAndNestedElementProvenance(t *testing.T) {
	left := nonZeroValue().withElement(zeroValue())
	right := zeroValue().withElement(nonZeroValue())
	joined := left.join(right)
	if !joined.zero || !joined.nonZero || joined.opaque || joined.elem == nil || !joined.elem.zero || !joined.elem.nonZero {
		t.Fatalf("joined value = %#v, want both outer and element states", joined)
	}
	if joined.equal(left) || joined.equal(right) {
		t.Fatal("join collapsed distinct state alternatives")
	}
	if !sameSet(joinSet(map[siteID]bool{1: true}, map[siteID]bool{2: true}), map[siteID]bool{1: true, 2: true}) {
		t.Fatal("site provenance union lost one side")
	}
}
