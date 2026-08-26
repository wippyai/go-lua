package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	fixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

type rowPostingObservation struct {
	row      model.RowID
	key      uint64
	regionID guard.FormulaID
}

func TestKeyedLookupRowUsesOwnerDirectoryInverse(t *testing.T) {
	world := fixture.New(t)
	owned, ok := world.LeftRoot().Index(world.LayoutLeftKey())
	if !ok || !owned.Available() {
		t.Fatal("left keyed index")
	}
	borrowed, ok := owned.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("borrow keyed index")
	}

	for _, wantRow := range world.RowsLeft() {
		want := make([]rowPostingObservation, 0)
		completed, valid := borrowed.Scan(func(match index.Match) bool {
			if match.Row() != wantRow {
				return true
			}
			regionID, regionOK := match.Region().Identity()
			if !regionOK {
				t.Fatal("scan region identity")
			}
			want = append(want, rowPostingObservation{row: match.Row(), key: uint64(match.Key()), regionID: regionID})
			return true
		})
		if !completed || !valid || len(want) == 0 {
			t.Fatalf("scan row=%v result=(%v,%v) count=%d", wantRow, completed, valid, len(want))
		}

		got := make([]rowPostingObservation, 0, len(want))
		completed, valid = borrowed.LookupRow(wantRow, func(match index.Match) bool {
			regionID, regionOK := match.Region().Identity()
			if !regionOK {
				t.Fatal("lookup region identity")
			}
			got = append(got, rowPostingObservation{row: match.Row(), key: uint64(match.Key()), regionID: regionID})
			return true
		})
		if !completed || !valid || len(got) != len(want) {
			t.Fatalf("lookup row=%v result=(%v,%v) count=%d want=%d", wantRow, completed, valid, len(got), len(want))
		}
		for position := range want {
			if got[position] != want[position] {
				t.Fatalf("lookup posting %d=%v want %v", position, got[position], want[position])
			}
		}
	}

	// The owner directory may authenticate a row before its keyed support is
	// published. That is a valid empty result, not a missing RowID inverse.
	empty, ok := world.Base().Index(world.LayoutLeftKey())
	if !ok || !empty.Available() {
		t.Fatal("base keyed index")
	}
	emptyBorrowed, ok := empty.Borrow()
	if !ok || !emptyBorrowed.Available() {
		t.Fatal("borrow base keyed index")
	}
	count := 0
	completed, valid := emptyBorrowed.LookupRow(world.RowsLeft()[0], func(index.Match) bool {
		count++
		return true
	})
	if !completed || !valid || count != 0 {
		t.Fatalf("empty keyed lookup=(%v,%v), callbacks=%d", completed, valid, count)
	}

	stale, ok := model.IssueRowID(world.RelationLeft(), identity.ContentID{0x7a})
	if !ok {
		t.Fatal("stale row")
	}
	for _, invalid := range []model.RowID{stale, world.RowsRight()[0]} {
		if completed, valid := borrowed.LookupRow(invalid, func(index.Match) bool { return true }); completed || valid {
			t.Fatalf("invalid row %v redeemed=(%v,%v)", invalid, completed, valid)
		}
	}
}

func TestKeyedLookupRowVisitorStopIsIncompleteButValid(t *testing.T) {
	world := fixture.New(t)
	owned, ok := world.LeftRoot().Index(world.LayoutLeftKey())
	if !ok || !owned.Available() {
		t.Fatal("left keyed index")
	}
	borrowed, ok := owned.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("borrow keyed index")
	}
	completed, valid := borrowed.LookupRow(world.RowsLeft()[0], func(index.Match) bool { return false })
	if completed || !valid {
		t.Fatalf("visitor stop=(%v,%v)", completed, valid)
	}
}
