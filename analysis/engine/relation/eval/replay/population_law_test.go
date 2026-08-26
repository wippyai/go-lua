package replay

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestPopulateRefusesUnboundCapabilities(t *testing.T) {
	called := false
	completed, valid := Populate(arrangement.ApplyReplay{}, witness.Mounted{}, read.Reader{}, func(CoordinateEvidence) bool {
		called = true
		return true
	})
	if completed || valid || called {
		t.Fatalf("zero replay admitted: completed=%v valid=%v called=%v", completed, valid, called)
	}
}

func TestCellAtIsExactBorrowedCoordinateAndEvidenceRejectsAbsent(t *testing.T) {
	fixture := testfixture.New(t)
	reader, ok := fixture.ReaderLeftPayload(fixture.LeftRoot())
	if !ok || !reader.Available() {
		t.Fatal("left input reader")
	}

	seen := 0
	completed, valid := reader.Scan(func(row read.Row) bool {
		seen++
		cells := row.Cells()
		if len(cells) == 0 {
			t.Fatal("empty full-vector row")
		}
		for index, want := range cells {
			got, gotOK := row.CellAt(index)
			if !gotOK || !got.Available() || got.Column() != want.Column() || got.Type() != want.Type() {
				t.Fatalf("CellAt(%d) mismatch: ok=%v got=%v want=%v", index, gotOK, got.Column(), want.Column())
			}
		}
		if _, gotOK := row.CellAt(-1); gotOK {
			t.Fatal("negative CellAt redeemed")
		}
		if _, gotOK := row.CellAt(len(cells)); gotOK {
			t.Fatal("out-of-range CellAt redeemed")
		}

		cell, cellOK := row.CellAt(0)
		if !cellOK {
			t.Fatal("coordinate cell")
		}
		evidence := CoordinateEvidence{
			row:         row.ID(),
			ordinal:     0,
			scope:       row.Scope(),
			lineage:     row.Lineage(),
			cellLineage: cell.Lineage(),
			value:       cell.Value(),
			presence:    cell.Presence(),
			fence:       fixture.Mounted().RuntimeFence(),
		}
		if !evidence.Available() {
			t.Fatal("valid coordinate evidence refused")
		}
		absent := evidence
		absent.presence, _ = model.NewPresence(model.ProvenAbsent)
		if absent.Available() {
			t.Fatal("proven-absent coordinate evidence admitted")
		}
		return true
	})
	if !completed || !valid || seen == 0 {
		t.Fatalf("row scan=(%v,%v), seen=%d", completed, valid, seen)
	}
}
