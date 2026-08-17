package project

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestProjectCountRowsMatchMountAndBaseCardinalities(t *testing.T) {
	p := projectProgram(t, `local value = 1; return value`)
	componentDraft := projectDraft(t, []Module{{Name: "main", Program: p}})
	component, err := componentDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	rows := component.CountRows()
	if !rows.Available() || rows.Count() != 2 {
		t.Fatalf("Project CountRows = %d/%t, want 2/true", rows.Count(), rows.Available())
	}
	ids := denominator.GeneratedLinkProjectIDs()
	want := []struct {
		id    schema.EntryID
		count int
	}{
		{ids.LinkProjectShardMount, component.Mounts().Count()},
		{ids.LinkProjectBaseApplication, component.Applications().Bases().Count()},
	}
	for _, row := range want {
		if got, ok := rows.Value(row.id); !ok || got != uint64(row.count) {
			t.Fatalf("Project denominator %v = %d/%t, want %d/true", row.id, got, ok, row.count)
		}
	}
}
