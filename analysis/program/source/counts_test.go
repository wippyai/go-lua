package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestCountRowsPublishesExactlyTheSourceOwnerRows(t *testing.T) {
	input, index := sourceFixture(2)
	component := finalizeSource(t, input, index)
	rows, err := CountRows(component.View())
	if err != nil {
		t.Fatalf("CountRows: %v", err)
	}
	if !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerProgramSource) {
		t.Fatal("source rows did not cover the generated owner catalog exactly")
	}
	ids := denominator.GeneratedProgramSourceIDs()
	if got, ok := rows.Value(ids.ProgramFlowLiterals); !ok || got != 4 {
		t.Fatalf("literal count = %d/%v, want 4/true", got, ok)
	}
	if got, ok := rows.Value(ids.ProgramFlowBody); !ok || got != 2 {
		t.Fatalf("body count = %d/%v, want 2/true", got, ok)
	}
}
