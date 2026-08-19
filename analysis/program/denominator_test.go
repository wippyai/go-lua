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
	if !denominator.GeneratedCountRowsCompleteForOwners(rows,
		denominator.RelationOwnerProgramSource,
		denominator.RelationOwnerProgramFlow,
		denominator.RelationOwnerProgramStatic,
		denominator.RelationOwnerProgramModule,
	) {
		t.Fatal("Program did not freeze the generated Program owner denominator")
	}
}
