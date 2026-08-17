package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestCandidateQueriesAreAllocationFree(t *testing.T) {
	result := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{unaryNumeric: []keyspace.Term{term(keyspace.FamilyUnary, 1)}},
		classes:  classStore{unaryClass: []uint8{unaryNumericCandidate}},
	}
	term := term(keyspace.FamilyUnary, 1)
	view := result.UnaryNumeric()
	if allocations := testing.AllocsPerRun(1000, func() {
		if view.Count() != 1 || !view.Contains(term) {
			t.Fatal("stable typed query returned an incorrect result")
		}
		_, _ = view.At(0)
	}); allocations != 0 {
		t.Fatalf("typed candidate query allocated %v objects per run", allocations)
	}
}
