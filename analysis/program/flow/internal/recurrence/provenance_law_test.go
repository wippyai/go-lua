package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSealRejectsEqualCardinalityForeignOwnerSplices(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	loop := term(keyspace.FamilyLoop, 1)
	base := ownerSpec{
		name:      "recurrence-provenance-a.lua",
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Loops: []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	}
	first := openOwnerFixture(t, base)

	foreignSourceSpec := base
	foreignSourceSpec.name = "recurrence-provenance-b.lua"
	foreignSource := openOwnerFixture(t, foreignSourceSpec)
	if first.sourceView.Identity().ContentID() == foreignSource.sourceView.Identity().ContentID() {
		t.Fatal("foreign Source fixtures unexpectedly share ContentID")
	}
	if _, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, foreignSource.graph,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("recurrence accepted an equal-cardinality foreign sourcecontrol Source splice")
	}
	if _, err := Seal(foreignSource.sourceView, first.flow, first.bodies, first.forest, first.graph,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("recurrence accepted an equal-cardinality foreign Source splice")
	}

	foreignFlowSpec := base
	foreignFlowSpec.nilOwners = []keyspace.Term{child}
	foreignFlowSpec.flow = authored.Input{Control: authored.ControlInput{
		Loops: []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopRepeat, Control: term(keyspace.FamilyNil, 1)}},
	}}
	foreignFlow := openOwnerFixture(t, foreignFlowSpec)
	if first.sourceView.Identity().ContentID() == foreignFlow.sourceView.Identity().ContentID() {
		t.Fatal("foreign Flow fixtures unexpectedly share Source identity")
	}
	if first.flow.Cold().ContentID() == foreignFlow.flow.Cold().ContentID() {
		t.Fatal("foreign Flow fixtures unexpectedly share ContentID")
	}
	if _, err := Seal(first.sourceView, foreignFlow.flow, first.bodies, first.forest, first.graph,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("recurrence accepted an equal-cardinality foreign Flow splice")
	}
}

func TestRecurrenceMatchesAndQueriesFailClosedWithoutOwnerIDs(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	loop := term(keyspace.FamilyLoop, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Loops: []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	})
	result, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	sourceID := fixture.sourceView.Identity().ContentID()
	flowID := fixture.flow.Cold().ContentID()
	staticID := fixture.staticFinalize.View().ContentID()
	moduleID := fixture.moduleFinalize.View().ContentID()
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("recurrence result did not retain its owner identities")
	}
	foreignSourceID := sourceID
	foreignSourceID[0]++
	foreignFlowID := flowID
	foreignFlowID[0]++
	foreignStaticID := staticID
	foreignStaticID[0]++
	foreignModuleID := moduleID
	foreignModuleID[0]++
	if Matches(result, foreignSourceID, flowID, staticID, moduleID) || Matches(result, sourceID, foreignFlowID, staticID, moduleID) ||
		Matches(result, sourceID, flowID, foreignStaticID, moduleID) || Matches(result, sourceID, flowID, staticID, foreignModuleID) ||
		Matches(result, identity.ContentID{}, flowID, staticID, moduleID) || Matches(result, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) || Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("recurrence provenance match accepted a foreign or unavailable identity")
	}

	zero := &Result{
		annotations: []Annotation{{Head: loop, Past: 1}},
		streams:     []keyspace.Term{loop},
	}
	if got := zero.ArcCount(); got != 0 {
		t.Fatalf("zero-ID ArcCount = %d, want 0", got)
	}
	if _, ok := zero.ArcAt(0); ok {
		t.Fatal("zero-ID ArcAt unexpectedly succeeded")
	}
	if got, ok := zero.ResetCount(0); ok || got != 0 {
		t.Fatalf("zero-ID ResetCount = %d/%v, want 0/false", got, ok)
	}
	if got, ok := zero.ResetAt(0, 0); ok || got != 0 {
		t.Fatalf("zero-ID ResetAt = %v/%v, want 0/false", got, ok)
	}
	if zero.ResetContains(0, loop) {
		t.Fatal("zero-ID ResetContains unexpectedly succeeded")
	}
	if got, ok := zero.DecisionCount(loop); ok || got != 0 {
		t.Fatalf("zero-ID DecisionCount = %d/%v, want 0/false", got, ok)
	}
	if got, ok := zero.DecisionAt(loop, 0); ok || got != 0 {
		t.Fatalf("zero-ID DecisionAt = %v/%v, want 0/false", got, ok)
	}
}
