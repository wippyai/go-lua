package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestResultMatchesExactFourOwnerIdentitiesAndQueriesFailClosed(t *testing.T) {
	fixture := openSemanticFixture(t, semanticSpec{
		counts: countsWith(familyCount{keyspace.FamilyBody, 1}),
		rows:   [][]keyspace.Term{{}},
	})
	result := fixture.result
	sourceID := fixture.sourceView.Identity().ContentID()
	flowID := fixture.flow.Cold().ContentID()
	staticID := fixture.staticFinalize.View().ContentID()
	moduleID := fixture.moduleFinalize.View().ContentID()
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("source-control Result did not match its exact four owners")
	}
	foreignSource := sourceID
	foreignSource[0] ^= 0xff
	foreignFlow := flowID
	foreignFlow[0] ^= 0xff
	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	if Matches(nil, sourceID, flowID, staticID, moduleID) ||
		Matches(result, foreignSource, flowID, staticID, moduleID) ||
		Matches(result, sourceID, foreignFlow, staticID, moduleID) ||
		Matches(result, sourceID, flowID, foreignStatic, moduleID) ||
		Matches(result, sourceID, flowID, staticID, foreignModule) ||
		Matches(result, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(result, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) ||
		Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("source-control provenance accepted nil, unavailable, or foreign owner")
	}

	zero := *result
	zero.moduleID = identity.ContentID{}
	if zero.NodeCount() != 0 || zero.ArcCount() != 0 {
		t.Fatalf("zero-provenance counts = nodes %d arcs %d, want 0/0", zero.NodeCount(), zero.ArcCount())
	}
	if _, ok := zero.Cursor(keyspace.MakeTerm(keyspace.FamilyBody, 1), 0); ok {
		t.Fatal("zero-provenance Cursor unexpectedly succeeded")
	}
	if _, ok := zero.Tail(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("zero-provenance Tail unexpectedly succeeded")
	}
	if _, ok := zero.Decision(keyspace.MakeTerm(keyspace.FamilyLoop, 1)); ok {
		t.Fatal("zero-provenance Decision unexpectedly succeeded")
	}
	if _, ok := zero.Coordinate(fixture.sourceView, keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("zero-provenance Coordinate unexpectedly succeeded")
	}
	if _, ok := zero.Resume(keyspace.MakeTerm(keyspace.FamilyLabel, 1)); ok {
		t.Fatal("zero-provenance Resume unexpectedly succeeded")
	}
	if _, ok := zero.ArcAt(0); ok {
		t.Fatal("zero-provenance ArcAt unexpectedly succeeded")
	}
	if zero.ArcCountAtSource(keyspace.MakeTerm(keyspace.FamilyBody, 1)) != 0 {
		t.Fatal("zero-provenance ArcCountAtSource unexpectedly succeeded")
	}
	if _, _, ok := zero.ArcAtSource(keyspace.MakeTerm(keyspace.FamilyBody, 1), 0); ok {
		t.Fatal("zero-provenance ArcAtSource unexpectedly succeeded")
	}
	if zero.SuccessorCount(0) != 0 || zero.PredecessorCount(0) != 0 || zero.Reachable(0) || zero.Dominates(0, 0) {
		t.Fatal("zero-provenance graph queries unexpectedly succeeded")
	}
	if _, ok := zero.SuccessorAt(0, 0); ok {
		t.Fatal("zero-provenance SuccessorAt unexpectedly succeeded")
	}
	if _, ok := zero.PredecessorAt(0, 0); ok {
		t.Fatal("zero-provenance PredecessorAt unexpectedly succeeded")
	}
}
