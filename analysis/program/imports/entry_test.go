package imports

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestEntryCommitCopiesCallerOwnedRows(t *testing.T) {
	entry := entryWithMember()
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       entry,
	})
	entry.Members[0].Suffix = 99
	entry.Roots[0] = 0
	view := component.View().Entry()
	field := entryWithMember().Members[0].Field
	if got, suffix, ok := view.MemberParent(field); !ok || got == 0 || suffix != 1 {
		t.Fatalf("MemberParent after caller mutation = %v/%d/%v, want retained parent/1/true", got, suffix, ok)
	}
	if got, ok := view.RootFunction(keyspace.MakeTerm(keyspace.FamilyReturn, 1), 0); ok || got != 0 {
		t.Fatalf("RootFunction unexpectedly retained a non-function slot = %v/%v", got, ok)
	}
}
