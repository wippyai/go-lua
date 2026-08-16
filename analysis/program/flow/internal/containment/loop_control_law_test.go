package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TestProveLoopControlsUseTheirTypedValueParent exercises the complete
// containment path with the smallest ordinary loop matrix.  While and Repeat
// controls are scalar value occurrences; NumericFor and GenericFor controls
// are Values occurrences.  Prove runs both flow emitters, so this keeps the
// control-family distinction at the boundary where a hardcoded FamilyValues
// check would reject the scalar forms.
func TestProveLoopControlsUseTheirTypedValueParent(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 5),
		c(keyspace.FamilyBool, 3),
		c(keyspace.FamilyNil, 2),
		c(keyspace.FamilyCell, 2),
		c(keyspace.FamilyValues, 2),
		c(keyspace.FamilyLoop, 4),
	)
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loops := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyLoop, 1),
		keyspace.MakeTerm(keyspace.FamilyLoop, 2),
		keyspace.MakeTerm(keyspace.FamilyLoop, 3),
		keyspace.MakeTerm(keyspace.FamilyLoop, 4),
	}
	controls := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyNil, 1),
		keyspace.MakeTerm(keyspace.FamilyNil, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyValues, 2),
	}

	rows := [][]keyspace.Term{{loops[0], loops[1], loops[2], loops[3]}}
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: entry, Fixed: authored.Range{End: 2}},
				{Owner: entry, Fixed: authored.Range{Start: 2, End: 3}},
			},
			Terms: []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyBool, 1),
				keyspace.MakeTerm(keyspace.FamilyBool, 2),
				keyspace.MakeTerm(keyspace.FamilyBool, 3),
			},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: keyspace.MakeTerm(keyspace.FamilyBody, 4)},
			{Kind: authored.CellLocal, Body: keyspace.MakeTerm(keyspace.FamilyBody, 5)},
		}},
		Control: authored.ControlInput{Loops: []authored.Loop{
			{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 2), Kind: kind.LoopWhile, Control: controls[0]},
			{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 3), Kind: kind.LoopRepeat, Control: controls[1]},
			{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 4), Kind: kind.LoopNumericFor, Control: controls[2], Cells: authored.Range{End: 1}},
			{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 5), Kind: kind.LoopGenericFor, Control: controls[3], Cells: authored.Range{Start: 1, End: 2}},
		}, Cells: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyCell, 1),
			keyspace.MakeTerm(keyspace.FamilyCell, 2),
		}},
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   rows,
		flow:   flow,
		nilOwners: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyBody, 1),
			keyspace.MakeTerm(keyspace.FamilyBody, 3),
		},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	for index, want := range []struct {
		name          string
		kind          kind.LoopKind
		loop          keyspace.Term
		control       keyspace.Term
		controlFamily keyspace.Family
	}{
		{name: "while scalar", kind: kind.LoopWhile, loop: loops[0], control: controls[0], controlFamily: keyspace.FamilyNil},
		{name: "repeat scalar", kind: kind.LoopRepeat, loop: loops[1], control: controls[1], controlFamily: keyspace.FamilyNil},
		{name: "numeric Values", kind: kind.LoopNumericFor, loop: loops[2], control: controls[2], controlFamily: keyspace.FamilyValues},
		{name: "generic Values", kind: kind.LoopGenericFor, loop: loops[3], control: controls[3], controlFamily: keyspace.FamilyValues},
	} {
		t.Run(want.name, func(t *testing.T) {
			owner, body, gotKind, gotControl, ok := fixture.flowView.Control().Loops().Get(want.loop)
			if !ok || owner != entry || body != keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+2)) || gotKind != want.kind || gotControl != want.control {
				t.Fatalf("Loop(%v) = owner %v body %v kind %v control %v ok %v", want.loop, owner, body, gotKind, gotControl, ok)
			}
			if got := keyspace.TermFamily(want.control); got != want.controlFamily {
				t.Fatalf("control %v family = %v, want %v", want.control, got, want.controlFamily)
			}
			if parent, ok := result.Parent(want.control); !ok || parent != want.loop {
				t.Fatalf("control %v parent = %v/%v, want Loop %v", want.control, parent, ok, want.loop)
			}
			if parent, ok := result.Parent(want.loop); !ok || parent != entry {
				t.Fatalf("Loop %v parent = %v/%v, want Entry %v", want.loop, parent, ok, entry)
			}
			if !result.Contains(want.loop, want.control) {
				t.Fatalf("Loop %v does not contain control %v", want.loop, want.control)
			}
		})
	}
}

func TestProveRepeatControlUsesLoopBodyOwner(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	nilControl := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: countsFor(
			c(keyspace.FamilyBody, 2),
			c(keyspace.FamilyNil, 1),
			c(keyspace.FamilyLoop, 1),
		),
		rows:      [][]keyspace.Term{{loop}, {}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{Control: authored.ControlInput{Loops: []authored.Loop{{
			Owner: entry, Body: body, Kind: kind.LoopRepeat, Control: nilControl,
		}}}},
		module: emptyModule(t),
	})
	result, err := fixture.prove()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if parent, ok := result.Parent(nilControl); !ok || parent != loop {
		t.Fatalf("Repeat control parent = %v/%v, want Loop %v", parent, ok, loop)
	}
	if parent, ok := result.Parent(loop); !ok || parent != entry {
		t.Fatalf("Repeat Loop parent = %v/%v, want Entry %v", parent, ok, entry)
	}
}
