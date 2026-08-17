package imports

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestEntryViewRetainsExactMemberOriginAndRejectsForeignRows(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       entryWithMember(),
	})
	view := component.View().Entry()
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	if got, ok := view.MemberAt(returned, 0); !ok || got != field {
		t.Fatalf("MemberAt = %v/%v, want %v/true", got, ok, field)
	}
	if got, ordinal, table, ok := view.MemberOrigin(field); !ok || got != returned || ordinal != 0 || table != keyspace.MakeTerm(keyspace.FamilyTable, 1) {
		t.Fatalf("MemberOrigin = %v/%d/%v/%v", got, ordinal, table, ok)
	}
	if _, ok := view.MemberAt(keyspace.MakeTerm(keyspace.FamilyReturn, 2), 0); ok {
		t.Fatal("MemberAt accepted a foreign Return")
	}
}
