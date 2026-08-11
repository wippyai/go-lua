package materialization

import "testing"

func TestRoleIsClosedAndOnlyRecentAdvances(t *testing.T) {
	for _, role := range []Role{Exact, Recent, Summary} {
		if !role.Valid() {
			t.Fatalf("valid role %d rejected", role)
		}
	}
	for _, role := range []Role{Invalid, Role(4)} {
		if role.Valid() {
			t.Fatalf("invalid role %d admitted", role)
		}
	}
	if next, ok := RecentToSummary(Recent); !ok || next != Summary {
		t.Fatal("recent did not advance to summary")
	}
	for _, role := range []Role{Invalid, Exact, Summary} {
		if _, ok := RecentToSummary(role); ok {
			t.Fatalf("non-recent role %d advanced", role)
		}
	}
}
