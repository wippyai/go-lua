package runtimeentry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestResultAndResumeRowsRequireTheirExactSealedParents(t *testing.T) {
	var result Result
	if entry, ok := result.Entry(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok || entry != 0 {
		t.Fatal("zero runtime-entry result resolved a Body")
	}
	if Matches(&result, flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)) {
		t.Fatal("zero runtime-entry result matched plausible owners")
	}
	if OwnsParents(&result, nil, nil, nil) {
		t.Fatal("zero runtime-entry result claimed absent parents")
	}
	var row OutcomeResumeRow
	if row.Available() || row.OwnedBy(&result, nil) || row.MatchesRoute(1, 2) {
		t.Fatal("zero Outcome resume row crossed its owner fence")
	}
	if from, to, ok := row.Endpoints(&result, nil); ok || from != (row.from) || to != (row.to) {
		t.Fatal("foreign Outcome resume row exposed endpoints")
	}
}
