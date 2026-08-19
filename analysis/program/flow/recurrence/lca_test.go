package recurrence

import "testing"

func TestOfflineLCAsAnswersNestedAndCrossTreeQueries(t *testing.T) {
	parents := []uint32{NoNode, 0, 0, 1}
	roots := []uint32{0, 0, 0, 0}
	left := []uint32{3, 1, 2}
	right := []uint32{2, 3, 3}
	want := []uint32{0, 1, 0}
	got := OfflineLCAs(parents, roots, left, right)
	if len(got) != len(want) {
		t.Fatalf("OfflineLCAs returned %v; want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("OfflineLCAs[%d] = %d; want %d", index, got[index], want[index])
		}
	}
	roots[3] = 1
	got = OfflineLCAs(parents, roots, left, right)
	if got[0] != NoNode || got[1] != NoNode {
		t.Fatalf("cross-root queries were collapsed: %v", got)
	}
}

func TestOfflineLCAsRejectsMalformedForestAndQueryShapes(t *testing.T) {
	if got := OfflineLCAs(nil, nil, nil, nil); got != nil {
		t.Fatal("empty forest was accepted")
	}
	if got := OfflineLCAs([]uint32{NoNode, 0}, []uint32{0}, nil, nil); got != nil {
		t.Fatal("mismatched root denominator was accepted")
	}
	if got := OfflineLCAs([]uint32{NoNode, 1}, []uint32{0, 0}, nil, nil); got != nil {
		t.Fatal("non-ancestral parent was accepted")
	}
}
