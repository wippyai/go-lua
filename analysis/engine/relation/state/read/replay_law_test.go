package read_test

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func orderedLeftRows(t *testing.T, fixture testfixture.Fixture) []model.RowID {
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

func TestReplayRowIDsMatchesFilteredScan(t *testing.T) {
	fixture := testfixture.New(t)
	reader, ok := fixture.ReaderLeftInput(fixture.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left input reader")
	}
	ids := orderedLeftRows(t, fixture)
	wanted := ids[1:]

	want := make([]read.Row, 0)
	completed, valid := reader.Scan(func(row read.Row) bool {
		if row.ID() == wanted[0] {
			want = append(want, row)
		}
		return true
	})
	if !completed || !valid || len(want) == 0 {
		t.Fatalf("filtered full scan=(%v,%v), rows=%d", completed, valid, len(want))
	}

	got := make([]read.Row, 0, len(want))
	completed, valid = reader.ReplayRowIDs(wanted, func(row read.Row) bool {
		got = append(got, row)
		return true
	})
	if !completed || !valid || len(got) != len(want) {
		t.Fatalf("replay=(%v,%v), rows=%d, want=%d", completed, valid, len(got), len(want))
	}
	for position := range want {
		if !read.Same(want[position], got[position]) {
			t.Fatalf("replay row %d differs from filtered Scan", position)
		}
	}
}

func TestReplayRowIDsRejectsUnauthenticatedOrderingAndKeepsEmptyValid(t *testing.T) {
	fixture := testfixture.New(t)
	reader, ok := fixture.ReaderLeftInput(fixture.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left input reader")
	}
	ids := orderedLeftRows(t, fixture)

	count := 0
	completed, valid := reader.ReplayRowIDs(nil, func(read.Row) bool {
		count++
		return true
	})
	if !completed || !valid || count != 0 {
		t.Fatalf("empty replay=(%v,%v), callbacks=%d", completed, valid, count)
	}

	stale, ok := model.IssueRowID(fixture.RelationLeft(), identity.ContentID{0x73})
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
			completed, valid := reader.ReplayRowIDs(test.ids, func(read.Row) bool {
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
	reader, ok := fixture.ReaderLeftInput(fixture.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left input reader")
	}
	ids := orderedLeftRows(t, fixture)
	completed, valid := reader.ReplayRowIDs(ids, func(read.Row) bool { return false })
	if completed || !valid {
		t.Fatalf("stopped replay=(%v,%v)", completed, valid)
	}
}
