package returnprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func returnProjectionOwners() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID) {
	return flowtest.ContentIDAt(1), flowtest.ContentIDAt(2), flowtest.ContentIDAt(3), flowtest.ContentIDAt(4)
}

func TestResultQueriesUseExactOwnerFenceAndOrderedValues(t *testing.T) {
	sourceID, flowID, staticID, moduleID := returnProjectionOwners()
	result := &Result{
		sourceID: sourceID,
		flowID:   flowID,
		staticID: staticID,
		moduleID: moduleID,
		sealed:   true,
		rows: []row{
			{},
			{outcome: keyspace.MakeTerm(keyspace.FamilyOutcome, 1), start: 0, end: 2},
		},
		values: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyValues, 1),
			keyspace.MakeTerm(keyspace.FamilyValues, 2),
		},
	}

	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("sealed Return projection rejected its exact owners")
	}
	outcome, count, ok := result.ForBody(keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if !ok || outcome != keyspace.MakeTerm(keyspace.FamilyOutcome, 1) || count != 2 {
		t.Fatalf("ForBody = %v, %d, %t; want canonical outcome and two alternatives", outcome, count, ok)
	}
	for index, want := range result.values {
		if got, ok := result.ValueAt(keyspace.MakeTerm(keyspace.FamilyBody, 1), index); !ok || got != want {
			t.Fatalf("ValueAt(%d) = %v, %t; want %v, true", index, got, ok, want)
		}
	}
	foreign := sourceID
	foreign[0]++
	if Matches(result, foreign, flowID, staticID, moduleID) {
		t.Fatal("foreign source owner crossed the Return projection fence")
	}
}

func TestResultRejectsMalformedRowsAndTerms(t *testing.T) {
	sourceID, flowID, staticID, moduleID := returnProjectionOwners()
	result := &Result{
		sourceID: sourceID,
		flowID:   flowID,
		staticID: staticID,
		moduleID: moduleID,
		sealed:   true,
		rows:     []row{{}, {outcome: keyspace.MakeTerm(keyspace.FamilyOutcome, 1), start: 1, end: 0}},
	}
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("sealed Return projection lost its owner fence")
	}
	if _, _, ok := result.ForBody(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("malformed Return projection row was queryable")
	}
	if _, ok := result.ValueAt(keyspace.MakeTerm(keyspace.FamilyBody, 1), -1); ok {
		t.Fatal("negative ValueAt index was accepted")
	}
	if _, ok := result.ValueAt(keyspace.MakeTerm(keyspace.FamilyOutcome, 1), 0); ok {
		t.Fatal("foreign-family Body coordinate was accepted")
	}
}
