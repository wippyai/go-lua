package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Wake columns are a set, not the traversal history of a physical tree. This
// pins the mount-time normal form so duplicate Input/delivery/key observations
// and an equivalent child visitation permutation cannot change scheduling or
// the sealed schedule digest.
func TestCanonicalScheduleColumnsDeduplicatesAndOrdersPhysicalObservations(t *testing.T) {
	owner, ok := model.IssueOwnerID(scheduleWakeToken(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, scheduleWakeToken(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	first, ok := model.IssueColumnID(relation, scheduleWakeToken(t, "first"))
	if !ok {
		t.Fatal("first column")
	}
	second, ok := model.IssueColumnID(relation, scheduleWakeToken(t, "second"))
	if !ok {
		t.Fatal("second column")
	}
	forward, forwardOK := canonicalScheduleColumns([]model.ColumnID{second, first, second, first})
	reversed, reversedOK := canonicalScheduleColumns([]model.ColumnID{first, second, first, second})
	if !forwardOK || !reversedOK || len(forward) != 2 || len(reversed) != 2 {
		t.Fatalf("canonical wakes = (%v,%v) / (%v,%v)", forwardOK, len(forward), reversedOK, len(reversed))
	}
	if compareColumn(forward[0], forward[1]) >= 0 || forward[0] != reversed[0] || forward[1] != reversed[1] {
		t.Fatalf("wake order differs by traversal: %v / %v", forward, reversed)
	}
}

func scheduleWakeToken(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/schedule-wake-law/v1", []byte(label))
	if !ok {
		t.Fatalf("token %q", label)
	}
	return value
}
