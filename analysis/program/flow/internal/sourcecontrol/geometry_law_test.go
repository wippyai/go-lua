package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestValidateLoopCoordinatesRequiresEveryOrdinal(t *testing.T) {
	result := &geometry{
		loopNodes: []uint32{noNode, 17, noNode},
	}
	result.counts[keyspace.FamilyLoop] = 2
	if err := validateLoopCoordinates(result); err == nil {
		t.Fatal("missing Loop ordinal passed coordinate completeness")
	}

	result.loopNodes[2] = 23
	if err := validateLoopCoordinates(result); err != nil {
		t.Fatalf("complete Loop coordinates rejected: %v", err)
	}

	result.loopNodes[1] = noNode
	if err := validateLoopCoordinates(result); err == nil {
		t.Fatal("missing first Loop ordinal passed coordinate completeness")
	}
}
