package flow

import "testing"

func TestOrderedRowIdentityUnionMergesCanonicalRows(t *testing.T) {
	rows := orderedRowIdentity[int]{
		less: func(a, b int) bool { return a < b },
		same: func(a, b int) bool { return a == b },
	}

	got := rows.Union([]int{1, 3, 5}, []int{2, 3, 4}, nil)
	want := []int{1, 2, 3, 4, 5}
	if !sameInts(got, want) {
		t.Fatalf("Union = %#v, want %#v", got, want)
	}

	evens := rows.Union([]int{1, 3, 5}, []int{2, 3, 4}, func(row int) (int, bool) {
		return row, row%2 == 0
	})
	if !sameInts(evens, []int{2, 4}) {
		t.Fatalf("filtered Union = %#v, want [2 4]", evens)
	}
}

func sameInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
