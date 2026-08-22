package dispatch

import (
	"testing"

	"github.com/wippyai/go-lua/domain/call"
)

func TestClassifyCalleeSetFromFinalCallValue(t *testing.T) {
	assert := func(top, open, complete bool, count int, want Completeness, wantCardinality uint32, wantCardinalityAvailable, wantAvailable bool) {
		t.Helper()
		fact, available := classifyCalleeSetState(top, open, complete, count)
		cardinality, cardinalityAvailable := fact.Cardinality()
		if available != wantAvailable || fact.Available() != wantAvailable || fact.Completeness() != want || cardinality != wantCardinality || cardinalityAvailable != wantCardinalityAvailable {
			t.Fatalf("fact = completeness:%d cardinality:%d/%t available:%t, want %d %d/%t available:%t", fact.Completeness(), cardinality, cardinalityAvailable, available, want, wantCardinality, wantCardinalityAvailable, wantAvailable)
		}
	}

	assert(false, false, true, 1, Complete, 1, true, true)
	assert(false, false, true, 2, Incomplete, 2, true, true)
	assert(false, true, false, 2, Unknown, 0, false, true)
	assert(false, true, false, 0, Unknown, 0, false, true)
	assert(true, false, false, 0, Unknown, 0, false, true)
	assert(false, false, true, 0, InvalidCompleteness, 0, false, false)
	assert(false, false, false, 0, InvalidCompleteness, 0, false, false)
	assert(false, false, true, -1, InvalidCompleteness, 0, false, false)

	if fact, available := ClassifyCalleeSet(call.Value{}); available || fact.Available() {
		t.Fatal("empty Call cell published a callee-set conclusion")
	}
}
