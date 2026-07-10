package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDeadAssignmentProofsFindAllPathOverwriteFrontier(t *testing.T) {
	result, err := CheckFunction(parseFunction(t, `
function f(flag: boolean): number
local x = 1
if flag then
    x = 2
else
    x = 3
end
return x
end
`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	proof, ok := deadAssignmentProofByName(result.DeadAssignmentProofs(), "x")
	if !ok {
		t.Fatalf("missing dead-assignment proof for x")
	}
	if len(proof.Overwrites) != 2 {
		t.Fatalf("overwrite frontier len = %d, want 2", len(proof.Overwrites))
	}
	if len(proof.Exits) != 0 {
		t.Fatalf("exit frontier len = %d, want 0", len(proof.Exits))
	}
}

func TestDeadAssignmentProofsRejectPathWithReadBeforeOverwrite(t *testing.T) {
	result, err := CheckFunction(parseFunction(t, `
function f(flag: boolean): number
local x = 1
if flag then
    local y = x
else
    x = 2
end
return x
end
`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}

	if _, ok := deadAssignmentProofByName(result.DeadAssignmentProofs(), "x"); ok {
		t.Fatalf("unexpected dead-assignment proof for x")
	}
}

func deadAssignmentProofByName(proofs []DeadAssignmentProof, name string) (DeadAssignmentProof, bool) {
	for _, proof := range proofs {
		if proof.Write.Name == name {
			return proof, true
		}
	}
	return DeadAssignmentProof{}, false
}
