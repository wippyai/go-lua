package read_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	fixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestKeyedReaderLookupRowIDMatchesExactScanRows(t *testing.T) {
	world := fixture.New(t)
	reader, ok := world.ReaderLeftKey(world.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left keyed reader")
	}
	for _, wantID := range world.RowsLeft() {
		want := make([]read.Row, 0)
		completed, valid := reader.Scan(func(row read.Row) bool {
			if row.ID() == wantID {
				want = append(want, row)
			}
			return true
		})
		if !completed || !valid || len(want) == 0 {
			t.Fatalf("scan row=%v result=(%v,%v) count=%d", wantID, completed, valid, len(want))
		}

		got := make([]read.Row, 0, len(want))
		completed, valid = reader.LookupRowID(wantID, func(row read.Row) bool {
			got = append(got, row)
			return true
		})
		if !completed || !valid || len(got) != len(want) {
			t.Fatalf("lookup row=%v result=(%v,%v) count=%d want=%d", wantID, completed, valid, len(got), len(want))
		}
		for position := range want {
			if !read.Same(want[position], got[position]) {
				t.Fatalf("lookup row %d differs from Scan", position)
			}
		}
	}

	baseReader, ok := world.ReaderLeftKey(world.Base())
	if !ok || !baseReader.Available() {
		t.Fatal("base keyed reader")
	}
	count := 0
	completed, valid := baseReader.LookupRowID(world.RowsLeft()[0], func(read.Row) bool {
		count++
		return true
	})
	if !completed || !valid || count != 0 {
		t.Fatalf("empty keyed lookup=(%v,%v), callbacks=%d", completed, valid, count)
	}

	stale, ok := model.IssueRowID(world.RelationLeft(), identity.ContentID{0x7b})
	if !ok {
		t.Fatal("stale row")
	}
	for _, invalid := range []model.RowID{stale, world.RowsRight()[0]} {
		if completed, valid := reader.LookupRowID(invalid, func(read.Row) bool { return true }); completed || valid {
			t.Fatalf("invalid row %v redeemed=(%v,%v)", invalid, completed, valid)
		}
	}
}

func TestKeyedReaderLookupRowIDVisitorStopIsIncompleteButValid(t *testing.T) {
	world := fixture.New(t)
	reader, ok := world.ReaderLeftKey(world.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left keyed reader")
	}
	completed, valid := reader.LookupRowID(world.RowsLeft()[0], func(read.Row) bool { return false })
	if completed || !valid {
		t.Fatalf("visitor stop=(%v,%v)", completed, valid)
	}
}
