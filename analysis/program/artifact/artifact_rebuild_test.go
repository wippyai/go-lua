package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestArtifactRebuildDenominatorJoinRejectsMismatchedOwners(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyValues] = 1
	if ownerDenominatorsAgree(counts, flow.Input{}, static.Input{}, imports.Input{}) {
		t.Fatal("rebuild admitted a Flow denominator mismatch")
	}
	flowCounts := flowCounts(counts, flow.Input{Values: flow.ValuesInput{Rows: make([]flow.Value, 1)}})
	if flowCounts[keyspace.FamilyValues] != 1 {
		t.Fatalf("flow denominator = %d, want 1", flowCounts[keyspace.FamilyValues])
	}
}
