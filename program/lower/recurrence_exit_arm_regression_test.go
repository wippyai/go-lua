package lower_test

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
)

// TestRecurrenceExitArmLowersThroughEvaluationPorts keeps the real fixture
// that exposed the Repeat-control owner frontier on the named regression
// path. The fixture contains nested Repeat conditions authored by the loop
// Body; lowering must seal those ports without weakening the owner proof.
func TestRecurrenceExitArmLowersThroughEvaluationPorts(t *testing.T) {
	text, err := os.ReadFile("../../testdata/fixtures/soundness/recurrence-exit-arm/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(programlower.Source{
		Name: "testdata/fixtures/soundness/recurrence-exit-arm/main.lua",
		Text: text,
	})
	if err != nil {
		t.Fatal(err)
	}
	flow := p.Flow()
	loops := flow.Authored().Control().Loops()
	repeatCount := 0
	for ordinal := 0; ordinal < loops.Count(); ordinal++ {
		loop, ok := loops.At(ordinal)
		if !ok {
			t.Fatalf("missing Loop %d", ordinal+1)
		}
		owner, body, loopKind, control, ok := loops.Get(loop)
		if !ok {
			t.Fatalf("Loop %v has no authored row", loop)
		}
		if loopKind != kind.LoopRepeat {
			continue
		}
		repeatCount++
		if owner == body || keyspace.TermFamily(control) != keyspace.FamilyBinary {
			t.Fatalf("Repeat %v owner/body/control = %v/%v/%v; want distinct Body-owned Binary control", loop, owner, body, control)
		}
		binaryOwner, _, left, right, ok := flow.Authored().Operators().Binaries().Get(control)
		if !ok || binaryOwner != body || left == 0 || right == 0 {
			t.Fatalf("Repeat %v control %v owner/operands = %v/%v/%v/%v; want Body %v", loop, control, binaryOwner, left, right, ok, body)
		}
		if entry, ok := flow.Ports().Entry(loop); !ok || entry != body {
			t.Fatalf("Repeat %v Entry = %v/%v; want loop Body %v", loop, entry, ok, body)
		}
		if finish, ok := flow.Ports().Finish(loop); !ok || finish != loop {
			t.Fatalf("Repeat %v Finish = %v/%v; want Loop", loop, finish, ok)
		}
		controlEntry, controlOK := flow.Ports().Entry(control)
		leftEntry, leftOK := flow.Ports().Entry(left)
		if !controlOK || !leftOK || controlEntry != leftEntry {
			t.Fatalf("Repeat %v control/left Entry = %v/%v and %v/%v; want same evaluated port", loop, controlEntry, controlOK, leftEntry, leftOK)
		}
		if finish, ok := flow.Ports().Finish(control); !ok || finish != control {
			t.Fatalf("Repeat %v control Finish = %v/%v; want control", loop, finish, ok)
		}
	}
	if repeatCount != 1 {
		t.Fatalf("recurrence-exit-arm Repeat count = %d, want one frozen control frontier", repeatCount)
	}
}
