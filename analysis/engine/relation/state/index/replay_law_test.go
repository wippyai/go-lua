package index_test

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func orderedIndexRows(t *testing.T, fixture testfixture.Fixture) []model.RowID {
	t.Helper()
	rows := fixture.RowsLeft()
	result := []model.RowID{rows[0], rows[1]}
	sort.Slice(result, func(left, right int) bool {
		leftIndex, leftOK := fixture.Mounted().RowIndex(fixture.RelationLeft(), result[left])
		rightIndex, rightOK := fixture.Mounted().RowIndex(fixture.RelationLeft(), result[right])
		if !leftOK || !rightOK {
			t.Fatalf("row directory order")
		}
		return leftIndex < rightIndex
	})
	return result
}

func TestReplayRowIDsMatchesFilteredIndexScan(t *testing.T) {
	fixture := testfixture.New(t)
	owned, ok := fixture.LeftRoot().Index(fixture.LayoutInput())
	if !ok || !owned.Available() {
		t.Fatal("left input index")
	}
	borrowed, ok := owned.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("borrow left input index")
	}
	ids := orderedIndexRows(t, fixture)
	wanted := ids[1:]

	want := make([]model.RowID, 0)
	completed, valid := borrowed.Scan(func(match index.Match) bool {
		if match.Row() == wanted[0] {
			want = append(want, match.Row())
		}
		return true
	})
	if !completed || !valid || len(want) == 0 {
		t.Fatalf("filtered scan=(%v,%v), rows=%d", completed, valid, len(want))
	}

	got := make([]model.RowID, 0, len(want))
	completed, valid = borrowed.ReplayRowIDs(wanted, func(match index.Match) bool {
		got = append(got, match.Row())
		return true
	})
	if !completed || !valid || len(got) != len(want) {
		t.Fatalf("replay=(%v,%v), rows=%d, want=%d", completed, valid, len(got), len(want))
	}
	for position := range want {
		if got[position] != want[position] {
			t.Fatalf("replay row %d=%v, want %v", position, got[position], want[position])
		}
	}
}

func TestReplayRowIDsRejectsInvalidOrderingAndSupportsEmpty(t *testing.T) {
	fixture := testfixture.New(t)
	owned, ok := fixture.LeftRoot().Index(fixture.LayoutInput())
	if !ok || !owned.Available() {
		t.Fatal("left input index")
	}
	ids := orderedIndexRows(t, fixture)
	borrowed, ok := owned.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("borrow left input index")
	}

	count := 0
	completed, valid := borrowed.ReplayRowIDs(nil, func(index.Match) bool {
		count++
		return true
	})
	if !completed || !valid || count != 0 {
		t.Fatalf("empty replay=(%v,%v), callbacks=%d", completed, valid, count)
	}

	stale, ok := model.IssueRowID(fixture.RelationLeft(), identity.ContentID{0x74})
	if !ok {
		t.Fatal("stale row")
	}
	foreign := fixture.RowsRight()[0]
	cases := []struct {
		name string
		ids  []model.RowID
	}{
		{name: "foreign", ids: []model.RowID{foreign}},
		{name: "stale", ids: []model.RowID{stale}},
		{name: "duplicate", ids: []model.RowID{ids[0], ids[0]}},
		{name: "unsorted", ids: []model.RowID{ids[1], ids[0]}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			count := 0
			completed, valid := borrowed.ReplayRowIDs(test.ids, func(index.Match) bool {
				count++
				return true
			})
			if completed || valid || count != 0 {
				t.Fatalf("invalid replay=(%v,%v), callbacks=%d", completed, valid, count)
			}
		})
	}
}

func TestReplayRowIDsVisitorStopIsIncompleteButValid(t *testing.T) {
	fixture := testfixture.New(t)
	owned, ok := fixture.LeftRoot().Index(fixture.LayoutInput())
	if !ok || !owned.Available() {
		t.Fatal("left input index")
	}
	borrowed, ok := owned.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("borrow left input index")
	}
	ids := orderedIndexRows(t, fixture)
	completed, valid := borrowed.ReplayRowIDs(ids, func(index.Match) bool { return false })
	if completed || !valid {
		t.Fatalf("stopped replay=(%v,%v)", completed, valid)
	}
}
