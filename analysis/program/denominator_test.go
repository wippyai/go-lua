package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestProgramCountRowsUseGeneratedOwnerIdentities(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-counts-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	rows := published.CountRows()
	if !rows.Available() || rows.Count() == 0 {
		t.Fatal("Program did not freeze denominator count rows")
	}
	expected := 0
	for _, entry := range denominator.GeneratedRelationEntries() {
		if entry == nil {
			t.Fatal("generated denominator contained nil entry")
		}
		switch entry.Owner() {
		case denominator.RelationOwnerProgramSource,
			denominator.RelationOwnerProgramFlow,
			denominator.RelationOwnerProgramStatic,
			denominator.RelationOwnerProgramModule:
			expected++
			if _, ok := rows.Value(entry.ID()); !ok {
				t.Fatalf("missing frozen count for generated entry %v", entry.ID())
			}
		}
	}
	if rows.Count() != expected {
		t.Fatalf("frozen count rows = %d, want generated Program owner count %d", rows.Count(), expected)
	}
}
