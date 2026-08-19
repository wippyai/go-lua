package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestWideFamilyBaseLookupIsIterativeAndAllocationFree(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 4096
	counts[keyspace.FamilyOutcome] = 1
	r := &Result{
		index:    successorIndex{familyCounts: counts},
		sourceID: identity.ContentID{1},
		flowID:   identity.ContentID{2},
		staticID: identity.ContentID{3},
		moduleID: identity.ContentID{4},
	}
	high := keyspace.MakeTerm(keyspace.FamilyBody, 4096)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: high, To: outcome}}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := r.Successors().Count(high); got != 1 {
		t.Fatalf("wide family successor count = %d, want 1", got)
	}
	view := r.Successors()
	if allocs := testing.AllocsPerRun(1000, func() { _, _ = view.At(high, 0) }); allocs != 0 {
		t.Fatalf("wide successor query allocates %v times", allocs)
	}
}
