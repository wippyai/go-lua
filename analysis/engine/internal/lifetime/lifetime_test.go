package lifetime

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestExecutionStampCellsAdmitOnlyTheirLiveStamp(t *testing.T) {
	var sequence GenerationSequence
	first, issued := sequence.Issue()
	second, reissued := sequence.Issue()
	if !issued || !reissued || !first.Precedes(second) || second != first.Next() {
		t.Fatal("generation sequence did not advance")
	}
	var cell Cell
	if cell.live().Available() || cell.Claim(0) || !cell.Claim(first) || cell.Claim(second) || cell.Claim(first) {
		t.Fatal("generation cell accepted an invalid or duplicate holder")
	}
	if !cell.Holds(first) || cell.Holds(second) || cell.Revoke(second) || !cell.Revoke(first) || cell.Holds(first) {
		t.Fatal("generation cell did not enforce one live stamp")
	}
	cell.Open(second)
	next, advanced := cell.Advance()
	if !advanced || next != second.Next() || !cell.Holds(next) || cell.Holds(second) {
		t.Fatal("generation cell did not supersede its live stamp")
	}
}

func TestSequenceSaturationNeverWraps(t *testing.T) {
	var sequence Sequence[uint64]
	sequence.value.Store(^uint64(0) - 1)
	last, lastOK := sequence.Issue()
	if !lastOK || last != ^uint64(0) {
		t.Fatalf("last sequence value = %d/%v", last, lastOK)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if value, ok := sequence.Issue(); ok || value != 0 {
			t.Fatalf("exhausted sequence attempt %d = %d/%v", attempt, value, ok)
		}
	}
}

func TestCellAdvanceSaturationNeverWraps(t *testing.T) {
	var cell Cell
	cell.value.Store(^uint64(0) - 1)
	last, lastOK := cell.Advance()
	if !lastOK || last != identity.Generation(^uint64(0)) {
		t.Fatalf("last cell generation = %d/%v", last, lastOK)
	}
	if !cell.Holds(last) {
		t.Fatal("last cell generation was not retained")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if value, ok := cell.Advance(); ok || value.Available() {
			t.Fatalf("exhausted cell attempt %d = %d/%v", attempt, value, ok)
		}
		if !cell.Holds(last) {
			t.Fatal("cell saturation lost its live maximum")
		}
	}
}
