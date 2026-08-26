package step_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// A raw schedule entry is not an evaluator request. The entry must be
// redeemed from the exact mounted execution retained by Session; this keeps
// unchecked logical plans and foreign roots outside the evaluator surface.
func TestEvaluateRefusesUncheckedSessionAndEntry(t *testing.T) {
	var session step.Session
	if value, ok := session.Evaluate(arrangement.ScheduleEntry{}); ok || value.Available() {
		t.Fatal("unchecked evaluator request was accepted")
	}
}

// The first physical vertical proves the evaluator does not rediscover an
// access path: a dependency-owned schedule entry leads directly to the
// mounted Input binding, Reader, tuple batches, and canonical row identities.
func TestEvaluateRedeemsMountedInputEntry(t *testing.T) {
	fixture := testfixture.New(t, 0xD1)
	session, ok := step.New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("evaluator session")
	}
	execution := fixture.BothRoot().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.DependencyLeft())
	if !ok || !entry.Available() {
		t.Fatal("left schedule entry")
	}
	result, ok := session.Evaluate(entry)
	if !ok || !result.Available() || result.Kind() != algebra.KindInput || result.Dependency() != fixture.DependencyLeft() {
		t.Fatalf("input evaluation = (%v, %v, %v)", ok, result.Available(), result.Kind())
	}
	batches := result.Batches()
	if len(batches) == 0 {
		t.Fatal("input produced no cofiber batches")
	}
	seen := make(map[uint8]bool)
	rows := fixture.RowsLeft()
	for _, batch := range batches {
		if !batch.ValidFor(fixture.Mounted()) {
			t.Fatal("foreign input batch")
		}
		for _, value := range batch.Tuples() {
			row, rowOK := value.SourceFor(fixture.RelationLeft())
			if !rowOK {
				t.Fatal("input tuple lost source row")
			}
			for index, want := range rows {
				if row == want {
					seen[uint8(index)] = true
				}
			}
		}
	}
	if len(seen) != len(rows) {
		t.Fatalf("input rows seen=%d want=%d", len(seen), len(rows))
	}
}
