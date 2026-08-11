package control

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// provenanceShapeSpec keeps the source denominator fixed while changing only
// the authored order or loop kind. Both variants are honest seals; the
// identity fence, rather than a cardinality check, must distinguish them.
func provenanceShapeSpec(loopKind kind.LoopKind, reorder bool) shapeSpec {
	counts := controlCounts(2, 0, 1, 0, 0, 1, 1, 1, 1, 0)
	body, child := keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 2)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	gotoTerm := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	breakTerm := keyspace.MakeTerm(keyspace.FamilyBreak, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	first := []keyspace.Term{label, loop}
	if reorder {
		first = []keyspace.Term{loop, label}
	}
	nilOwner := body
	if loopKind == kind.LoopRepeat {
		nilOwner = child
	}
	return shapeSpec{
		counts: counts,
		rows:   bodyRows(first, []keyspace.Term{gotoTerm, breakTerm}),
		flow: authored.Input{
			Counts: counts,
			Control: authored.ControlInput{
				Labels: []authored.Label{{Owner: body}},
				Gotos:  []authored.Goto{{Owner: child, Target: label}},
				Breaks: []authored.Break{{Owner: child}},
				Loops:  []authored.Loop{{Owner: body, Body: child, Kind: loopKind, Control: nilTerm}},
			},
		},
		nilOwners: []keyspace.Term{nilOwner},
	}
}

func TestShapeProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	current := openShapeFixture(t, provenanceShapeSpec(kind.LoopWhile, false))
	foreignSource := openShapeFixture(t, provenanceShapeSpec(kind.LoopWhile, true))
	foreignFlow := openShapeFixture(t, provenanceShapeSpec(kind.LoopRepeat, false))

	shape := current.seal(t)
	sourceID := current.preimage.Identity().ContentID()
	flowID := current.flow.Cold().ContentID()
	staticID := current.staticFinalize.View().ContentID()
	moduleID := current.moduleFinalize.View().ContentID()
	if !Matches(shape, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Shape did not retain its exact Source/Flow identities")
	}
	if sourceID == foreignSource.preimage.Identity().ContentID() ||
		flowID == foreignFlow.flow.Cold().ContentID() {
		t.Fatal("foreign fixtures did not preserve equal denominators with distinct identities")
	}
	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	if Matches(shape, foreignSource.preimage.Identity().ContentID(), flowID, staticID, moduleID) ||
		Matches(shape, sourceID, foreignFlow.flow.Cold().ContentID(), staticID, moduleID) ||
		Matches(shape, sourceID, flowID, foreignStatic, moduleID) ||
		Matches(shape, sourceID, flowID, staticID, foreignModule) ||
		Matches(foreignSource.seal(t), sourceID, flowID, staticID, moduleID) {
		t.Fatal("Shape provenance accepted a foreign owner")
	}

	if _, err := Seal(current.preimage, current.flow, foreignSource.bodies, current.binding, current.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Body result")
	}
	if _, err := Seal(current.preimage, current.flow, current.bodies, current.binding, foreignSource.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Containment result")
	}
	if _, err := Seal(current.preimage, current.flow, current.bodies, foreignFlow.binding, current.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Binding result")
	}
}

func TestShapeProvenanceFailsClosedForNilAndZero(t *testing.T) {
	counts := controlCounts(1, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	shape := openShapeFixture(t, shapeSpec{
		counts: counts,
		rows:   bodyRows(nil),
		flow:   authored.Input{Counts: counts},
	}).seal(t)
	sourceID := shape.sourceID
	flowID := shape.flowID
	staticID := shape.staticID
	moduleID := shape.moduleID
	if Matches(nil, sourceID, flowID, staticID, moduleID) || Matches(shape, keyspace.ContentID{}, flowID, staticID, moduleID) ||
		Matches(shape, sourceID, keyspace.ContentID{}, staticID, moduleID) ||
		Matches(shape, sourceID, flowID, keyspace.ContentID{}, moduleID) || Matches(shape, sourceID, flowID, staticID, keyspace.ContentID{}) {
		t.Fatal("Shape Matches did not fail closed for nil or zero identity")
	}
	zero := &Shape{labelBody: shape.labelBody, breakLoop: shape.breakLoop, gotoTargetBody: shape.gotoTargetBody}
	if Matches(zero, sourceID, flowID, staticID, moduleID) {
		t.Fatal("zero-provenance Shape bypassed Matches")
	}
	if got, ok := zero.LabelBody(keyspace.MakeTerm(keyspace.FamilyLabel, 1)); ok || got != 0 {
		t.Fatalf("zero-provenance Shape query = %v/%v, want zero/false", got, ok)
	}
}
