package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestClassifyArcRejectsMalformedLoopEndpoint(t *testing.T) {
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
		flow: authored.Input{Control: authored.ControlInput{Loops: []authored.Loop{{
			Owner: parent, Body: child, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1),
		}}}},
	})
	parts, err := deriveComponents(fixture.graph)
	if err != nil {
		t.Fatalf("deriveComponents: %v", err)
	}
	trace, err := buildEventTrace(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph, parts)
	if err != nil {
		t.Fatalf("buildEventTrace: %v", err)
	}
	sealedCounts, err := validateOwners(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph)
	if err != nil {
		t.Fatalf("validateOwners: %v", err)
	}
	var feedback sourcecontrol.Arc
	found := false
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		candidate, ok := fixture.graph.ArcAt(index)
		if ok && candidate.Source == child && candidate.Target == loop && candidate.Decision == 0 {
			feedback, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("owner fixture has no Loop feedback Arc")
	}
	feedback.From = (feedback.From + 1) % fixture.graph.NodeCount()
	if _, _, recurrent, err := classifyArc(fixture.sourceView, fixture.flow, fixture.graph, trace, fixture.flow.Control().Loops(), feedback, sealedCounts); err == nil || recurrent {
		t.Fatalf("malformed Loop feedback accepted: recurrent=%v err=%v", recurrent, err)
	}
}

func TestClassifyArcRejectsMalformedGotoEndpoint(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 1),
		familyCount(keyspace.FamilyLabel, 1),
		familyCount(keyspace.FamilyGoto, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	label := term(keyspace.FamilyLabel, 1)
	gotoTerm := term(keyspace.FamilyGoto, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{label, gotoTerm}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: parent, Target: label}},
		}},
	})
	parts, err := deriveComponents(fixture.graph)
	if err != nil {
		t.Fatalf("deriveComponents: %v", err)
	}
	trace, err := buildEventTrace(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph, parts)
	if err != nil {
		t.Fatalf("buildEventTrace: %v", err)
	}
	sealedCounts, err := validateOwners(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph)
	if err != nil {
		t.Fatalf("validateOwners: %v", err)
	}
	var backward sourcecontrol.Arc
	found := false
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		candidate, ok := fixture.graph.ArcAt(index)
		if ok && candidate.Source == gotoTerm && candidate.Target == label && candidate.Decision == 0 {
			backward, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("owner fixture has no Goto feedback Arc")
	}
	backward.To = (backward.To + 1) % fixture.graph.NodeCount()
	if _, _, recurrent, err := classifyArc(fixture.sourceView, fixture.flow, fixture.graph, trace, fixture.flow.Control().Loops(), backward, sealedCounts); err == nil || recurrent {
		t.Fatalf("malformed Goto Arc accepted: recurrent=%v err=%v", recurrent, err)
	}
}
