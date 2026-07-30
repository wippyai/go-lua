package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestCompleteReturnCandidatesRequiresEverySlot(t *testing.T) {
	complete := append(returnCandidate("left", "shape/target/v1/a", "scalar/nil"), returnCandidate("right", "scalar/nil", "shape/target/v1/b")...)
	tuples, ok := completeReturnCandidates(complete)
	if !ok || len(tuples) != 2 || len(tuples[0]) != 2 || string(tuples[1][1]) != "shape/target/v1/b" {
		t.Fatalf("complete candidates = %#v, %v", tuples, ok)
	}
	incomplete := []equation.Fact{
		{Key: "return-candidate/left/arity", Value: []byte("2")},
		{Key: "return-candidate/left/0", Value: []byte("scalar/nil")},
	}
	if tuples, ok := completeReturnCandidates(incomplete); ok || tuples != nil {
		t.Fatalf("partial candidates published a tuple: %#v", tuples)
	}
}
