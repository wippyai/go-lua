package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// TestProgramCountRowsUseGeneratedOwnerIdentities states the denominator column
// a published Program root freezes: exactly the three cold owners it holds.
// The ProgramModule family is not part of it, because the root holds no Module
// component; its authored module rows live behind Flow under the scalar
// ModuleID and their derived cardinalities are first sealed at the artifact
// boundary. The absence is asserted directly, so re-attaching a Module column
// to the root fails here instead of travelling as a second Module authority.
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
	) {
		t.Fatal("Program did not freeze the generated cold-owner denominator")
	}
	moduleIDs := denominator.GeneratedProgramModuleOwnerIDs()
	if len(moduleIDs) == 0 {
		t.Fatal("the ProgramModule owner issued no generated identities")
	}
	for _, id := range moduleIDs {
		if _, published := rows.Value(id); published {
			t.Fatalf("Program root published ProgramModule relation %v; that family is sealed at the artifact boundary", id)
		}
	}
}
