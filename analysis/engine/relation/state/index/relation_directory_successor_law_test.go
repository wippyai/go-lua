package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	relationfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

// TestRelationDirectorySuccessorTracksCommittedRows pins the delta path for
// the zero-width relation layout. A cold base index is empty, while the
// already-committed fixture successor enumerates the owner-issued rows. This
// catches the old defect where a directory layout had no declared payload
// columns and therefore never woke on its relation's semantic delta.
func TestRelationDirectorySuccessorTracksCommittedRows(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	layout := fixture.LayoutInput()
	base, baseOK := fixture.Base().Index(layout)
	left, leftOK := fixture.LeftRoot().Index(layout)
	both, bothOK := fixture.BothRoot().Index(layout)
	if !baseOK || !base.Available() || !leftOK || !left.Available() || !left.SuccessorOf(base) || !bothOK || !both.Available() || !both.SuccessorOf(left) {
		t.Fatal("relation directory successor")
	}
	baseCount, leftCount, bothCount := 0, 0, 0
	if completed, valid := base.Scan(func(index.Match) bool { baseCount++; return true }); !completed || !valid {
		t.Fatal("base relation scan")
	}
	if completed, valid := left.Scan(func(match index.Match) bool {
		leftCount++
		row, ok := fixture.Mounted().RowAt(fixture.RelationLeft(), int(match.Key()))
		if !ok || row != match.Row() {
			t.Fatal("directory row authority")
		}
		return true
	}); !completed || !valid {
		t.Fatal("successor relation scan")
	}
	if completed, valid := both.Scan(func(match index.Match) bool {
		bothCount++
		if match.Relation() != fixture.RelationLeft() {
			t.Fatal("foreign delta woke relation directory")
		}
		return true
	}); !completed || !valid {
		t.Fatal("foreign successor relation scan")
	}
	if baseCount != 0 || leftCount != len(fixture.RowsLeft()) || bothCount != leftCount {
		t.Fatalf("relation directory counts base=%d left=%d both=%d", baseCount, leftCount, bothCount)
	}
	if delta, ok := fixture.BaseToLeftDelta(); !ok || !delta.SemanticChanged() {
		t.Fatal("fixture semantic delta")
	}
}
