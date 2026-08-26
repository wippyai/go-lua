package binding_test

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// This law exercises the sealed row order and inverse together repeatedly.
// The allocation assertion is intentional: query paths borrow the sealed
// owner view and must not construct a replacement directory or scan helper.
func TestMembershipViewSealsOrderedIndexForRepeatedQueries(t *testing.T) {
	owner := issueOwner(t, "owner/membership-sealed")
	relation := issueRelation(t, owner, "relation/membership-sealed")
	const count = 128
	rows := make([]model.RowID, count)
	for index := range rows {
		rows[index] = mustRow(t, relation, "row/membership-sealed/"+strconv.Itoa(index))
	}
	view, ok := binding.NewMembershipView(relation, rows)
	if !ok || !view.Available() || view.Len() != count {
		t.Fatalf("sealed membership available=%t len=%d", view.Available(), view.Len())
	}

	for repeat := 0; repeat < 32; repeat++ {
		for index, want := range rows {
			got, gotOK := view.At(index)
			if !gotOK || got != want {
				t.Fatalf("ordered At(%d) = %v/%t, want %v/true", index, got, gotOK, want)
			}
			if !view.Contains(want) {
				t.Fatalf("membership query for row %d was false", index)
			}
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_, _ = view.At(count - 1)
		_ = view.Contains(rows[count-1])
	}); allocations != 0 {
		t.Fatalf("sealed membership queries allocated %f times", allocations)
	}

	rows[0] = mustRow(t, relation, "row/membership-sealed/mutated-source")
	if got, ok := view.At(0); !ok || got != mustRow(t, relation, "row/membership-sealed/0") {
		t.Fatal("sealed membership borrowed mutable source order")
	}
}
