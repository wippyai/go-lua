package arrangement_test

import (
	"testing"

	expandfixture "github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture/expand"
)

// Expand's reader is a hot runtime source, so its authored R columns must
// wake the dependency even though P remains cold frozen evidence. C is
// collected from the child Input path and therefore needs no separate Expand
// wake arm.
func TestExpandReaderColumnsAreExactScheduleWakes(t *testing.T) {
	fixture := expandfixture.New(t)
	execution := fixture.World().Mounted().Arrangement().Execution()
	entries := execution.WakeColumn(fixture.ReaderPayload1Column())
	if len(entries) != 1 || entries[0].Dependency() != fixture.Dependency() {
		t.Fatalf("reader payload wake entries=%d, want Expand dependency", len(entries))
	}
}

// Publisher identity is required to authenticate the frozen C→P evidence,
// but P is not a runtime relation.  It therefore cannot appear in the
// dependency read set or wake an Expand evaluation when its state changes.
func TestExpandPublisherIsColdEvidenceNotRuntimeWake(t *testing.T) {
	fixture := expandfixture.New(t)
	execution := fixture.World().Mounted().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.Dependency())
	if !ok || !entry.Available() {
		t.Fatal("Expand dependency entry unavailable")
	}
	for _, relation := range entry.Reads() {
		if relation == fixture.Contract().Publisher() {
			t.Fatal("Expand dependency retained cold publisher relation")
		}
	}
	if wakes := execution.WakeRelation(fixture.Contract().Publisher()); len(wakes) != 0 {
		t.Fatalf("publisher relation installed %d runtime wakes", len(wakes))
	}
}
