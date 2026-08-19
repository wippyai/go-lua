package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestCandidateQueriesAreAllocationFree(t *testing.T) {
	result := &Result{
		sourceID: flowtest.ContentIDAt(1),
		flowID:   flowtest.ContentIDAt(2),
		staticID: flowtest.ContentIDAt(3),
		moduleID: flowtest.ContentIDAt(4),
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
