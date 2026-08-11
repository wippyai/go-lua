package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// TestSealLexicalLabelHeadKeepsWhileInterval proves that a canonical head is
// only an SCC name.  The Label is lexically inside the while Body and is the
// minimum existing Label/Loop Term, so it names the SCC.  The while feedback
// interval nevertheless starts before the Loop's condition/event cluster,
// not at the Label's position or at a synthetic head node.
func TestSealLexicalLabelHeadKeepsWhileInterval(t *testing.T) {
	parent := term(keyspace.FamilyBody, 1)
	loopBody := term(keyspace.FamilyBody, 2)
	label := term(keyspace.FamilyLabel, 1)
	gotoTerm := term(keyspace.FamilyGoto, 1)
	loop := term(keyspace.FamilyLoop, 1)
	control := term(keyspace.FamilyNil, 1)

	fixture := openOwnerFixture(t, ownerSpec{
		counts: countsWith(
			familyCount(keyspace.FamilyBody, 2),
			familyCount(keyspace.FamilyNil, 1),
			familyCount(keyspace.FamilyLabel, 1),
			familyCount(keyspace.FamilyGoto, 1),
			familyCount(keyspace.FamilyLoop, 1),
		),
		rows:      [][]keyspace.Term{{loop}, {gotoTerm, label}},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: loopBody}},
			Gotos:  []authored.Goto{{Owner: loopBody, Target: label}},
			Loops:  []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: control}},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	if count, ok := recurrence.DecisionCount(label); !ok || count != 1 {
		t.Fatalf("Label head stream = %d/%v, want one/true", count, ok)
	}
	if got, ok := recurrence.DecisionAt(label, 0); !ok || got != loop {
		t.Fatalf("Label head decision = %v/%v, want Loop %v/true", got, ok, loop)
	}
	if _, ok := recurrence.DecisionCount(loop); ok {
		t.Fatal("while Loop became a second head instead of remaining an SCC member")
	}

	feedback := -1
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		arc, ok := fixture.graph.ArcAt(index)
		if ok && arc.Source == loopBody && arc.Target == loop && arc.Decision == 0 {
			feedback = index
			break
		}
	}
	if feedback < 0 {
		t.Fatal("sealed sourcecontrol graph has no while feedback witness")
	}
	annotation, ok := recurrence.ArcAt(feedback)
	if !ok {
		t.Fatalf("while feedback Arc %d has no recurrence annotation", feedback)
	}
	if annotation.Head != label || annotation.First != 0 || annotation.Past != 1 {
		t.Fatalf("while feedback annotation = %#v, want Label head with [0,1)", annotation)
	}
	if count, ok := recurrence.ResetCount(feedback); !ok || count != 1 || !recurrence.ResetContains(feedback, loop) {
		t.Fatalf("while feedback reset = %d/%v, want exactly Loop decision", count, ok)
	}
}
