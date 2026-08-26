package read_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

func TestChangeReaderScanChangesPublishesExactInsertPairs(t *testing.T) {
	fixture := testfixture.New(t)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok || !handle.Available() {
		t.Fatal("bind changes")
	}
	count := 0
	completed, valid := handle.ScanChanges(func(change read.RowChange) bool {
		count++
		if !change.Available() || !change.Base().Same(delta.Base()) || !change.Next().Same(delta.Next()) {
			t.Fatal("invalid transition roots")
		}
		if _, before := change.Before(); before {
			t.Fatal("insert unexpectedly has predecessor row")
		}
		after, present := change.After()
		if !present || after == nil || !after.Available() || after.ID() != change.ID() || !after.Scope().Same(change.Scope()) {
			t.Fatal("insert successor row missing")
		}
		return true
	})
	if !completed || !valid || count != len(fixture.RowsLeft()) {
		t.Fatalf("insert scan=(%v,%v), count=%d", completed, valid, count)
	}
}

func TestChangeReaderScanChangesPublishesReplacementPairs(t *testing.T) {
	fixture := testfixture.New(t)
	base := fixture.LeftRoot()
	columnID := fixture.PayloadColumnsLeft()[0]
	part := firstPart(t, fixture, base, columnID, fixture.RowsLeft()[0])
	value := part.Value()
	_, delta := publishCellUpdate(t, fixture, base, columnID, part, value, part.Presence(), freshLineage(t, "transition-replace"))
	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok || !handle.Available() {
		t.Fatal("bind replacement changes")
	}
	count := 0
	completed, valid := handle.ScanChanges(func(change read.RowChange) bool {
		count++
		before, beforePresent := change.Before()
		after, afterPresent := change.After()
		if !beforePresent || !afterPresent || before == nil || after == nil || before.ID() != after.ID() || before.ID() != change.ID() {
			t.Fatal("replacement did not pair rows")
		}
		beforeCells, afterCells := before.Cells(), after.Cells()
		if len(beforeCells) != len(afterCells) || len(afterCells) == 0 {
			t.Fatal("replacement row cells missing")
		}
		return true
	})
	if !completed || !valid || count == 0 {
		t.Fatalf("replacement scan=(%v,%v), count=%d", completed, valid, count)
	}
}
