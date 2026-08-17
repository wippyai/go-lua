package imports

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestCountRowsPublishesExactlyTheModuleOwnerRows(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       entryWithMember(),
	})
	rows, err := CountRows(component.View())
	if err != nil {
		t.Fatalf("CountRows: %v", err)
	}
	if !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerProgramModule) {
		t.Fatal("module rows did not cover the generated owner catalog exactly")
	}
	ids := denominator.GeneratedProgramModuleIDs()
	checks := []struct {
		id    schema.EntryID
		count uint64
	}{
		{ids.ProgramModuleImport, 2},
		{ids.ProgramModuleRequest, 2},
		{ids.ProgramModuleEntry, 1},
		{ids.ProgramModuleEntryRootCell, 0},
		{ids.ProgramModuleEntryMember, 1},
		{ids.ProgramModuleEntryRootFunction, 0},
	}
	for _, check := range checks {
		if got, ok := rows.Value(check.id); !ok || got != check.count {
			t.Fatalf("module row %v = %d/%v, want %d/true", check.id, got, ok, check.count)
		}
	}
}
