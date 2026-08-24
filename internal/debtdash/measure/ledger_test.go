package measure

import "testing"

// TestScheduledDeathRowsScopedToOneModule pins scheduledDeathRows to the
// module rooted at the given path: testdata/module-scope carries a real
// scheduled_death.go (2 rows), a dot-directory scratch clone with its own
// scheduled_death.go (3 rows), and a nested/ directory with its own go.mod
// and its own scheduled_death.go (3 rows). Only the 2 rows belonging to
// module-scope itself count; the dot-directory and the nested module are
// separate trees.
func TestScheduledDeathRowsScopedToOneModule(t *testing.T) {
	rows, err := scheduledDeathRows("testdata/module-scope")
	if err != nil {
		t.Fatalf("scheduledDeathRows: %v", err)
	}
	if rows != 2 {
		t.Errorf("scheduledDeathRows(testdata/module-scope) = %d, want 2", rows)
	}
}

// TestScheduledDeathRowsSkipsNestedTestdata pins the same scoping for a
// testdata directory found below the walk root: testdata/nested-fixture-data
// carries a real scheduled_death.go (2 rows) plus a vendor-app/testdata/
// tree with its own scheduled_death.go (5 rows) - a fixture tree for
// vendor-app's own tooling, not part of the worktree rooted here. Only the
// 2 rows outside vendor-app/testdata count.
func TestScheduledDeathRowsSkipsNestedTestdata(t *testing.T) {
	rows, err := scheduledDeathRows("testdata/nested-fixture-data")
	if err != nil {
		t.Fatalf("scheduledDeathRows: %v", err)
	}
	if rows != 2 {
		t.Errorf("scheduledDeathRows(testdata/nested-fixture-data) = %d, want 2", rows)
	}
}
