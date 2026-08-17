package schedule_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The solver schedule spells the bracket-stream vocabulary a third time, under
// its own names: an EventNode is the sealed member spelled "event/point". The
// package is deliberately dependency free in production, so this law lives in
// its external test package: it states the ordinal agreement without giving
// the scheduler an import.
//
// The agreement is load bearing because a schedule event ordinal and an
// artifact bracket ordinal are read against one another when a compiled graph
// binds a Region: a reordered member here would enter a recurrence where the
// artifact declared an exit.
func TestScheduleVocabularyIsTheSealedTable(t *testing.T) {
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	for _, member := range []struct {
		key      schema.Key
		ordinal  schedule.EventKind
		spelling string
	}{
		{"event/enter", schedule.EventEnter, "schedule.EventEnter"},
		{"event/point", schedule.EventNode, "schedule.EventNode"},
		{"event/exit", schedule.EventExit, "schedule.EventExit"},
	} {
		entry, ok := table.At(structure.CategoryEvent, uint16(member.ordinal))
		if !ok {
			t.Fatalf("%s = %d names no member of the sealed vocabulary", member.spelling, member.ordinal)
		}
		if entry.Key() != member.key {
			t.Fatalf("%s = %d is the sealed member %q, not %q", member.spelling, member.ordinal, entry.Key(), member.key)
		}
	}
	if int(schedule.EventExit) != table.Count(structure.CategoryEvent) {
		t.Fatalf("schedule.EventExit = %d, but the sealed vocabulary declares %d members", schedule.EventExit, table.Count(structure.CategoryEvent))
	}
}
