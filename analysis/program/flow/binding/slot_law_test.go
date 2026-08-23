package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TestSealPublishesHostOrderSlots pins the published slot column against the
// authored orders it is derived from. A consumer that needs a Cell's position
// inside its definition host reads this column; the authored Bind, formal,
// capture, and Loop orders are walked once, here.
func TestSealPublishesHostOrderSlots(t *testing.T) {
	terms := make([]keyspace.Term, 10)
	for index := range terms {
		terms[index] = keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
	}
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)

	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 10, keyspace.FamilyBind: 1,
		keyspace.FamilyValues: 1, keyspace.FamilyFunction: 1, keyspace.FamilyLoop: 1,
		keyspace.FamilyVararg: 1, keyspace.FamilyNil: 2,
	}}
	input.Values.Rows = []authored.Value{{Owner: body1, Fixed: authored.Range{Start: 0, End: 2}}}
	input.Values.Terms = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body1}, // 0: bind slot 1
		{Kind: authored.CellLocal, Body: body1}, // 1: bind slot 2
		{Kind: authored.CellLocal, Body: body1}, // 2: bind slot 3
		{Kind: authored.CellLocal, Body: body2}, // 3: formal slot 1
		{Kind: authored.CellLocal, Body: body2}, // 4: formal slot 2
		{Kind: authored.CellLocal, Body: body2}, // 5: Function Vararg, slot 0
		{Kind: authored.CellLocal, Body: body2}, // 6: capture slot 1
		{Kind: authored.CellLocal, Body: body2}, // 7: capture slot 2
		{Kind: authored.CellLocal, Body: body3}, // 8: Loop slot 1
		{Kind: authored.CellLocal, Body: body3}, // 9: Loop slot 2
	}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: terms[5]}}
	input.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: terms[5], Captures: authored.Range{Start: 0, End: 2}}}
	input.Functions.Captures = []authored.Capture{{Inner: terms[6], Outer: terms[0]}, {Inner: terms[7], Outer: terms[1]}}
	input.Control.Loops = []authored.Loop{{Owner: body1, Body: body3, Kind: kind.LoopGenericFor, Control: values, Cells: authored.Range{Start: 0, End: 2}}}
	input.Control.Cells = []keyspace.Term{terms[8], terms[9]}

	rows := [][]keyspace.Term{{bind, loop}, nil, nil}
	result, finish := sealLawFixture(t, input, rows,
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{terms[0], terms[1], terms[2]}}},
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{terms[3], terms[4]}}},
		nil,
	)
	defer finish()

	wantRoles := []kind.CellRole{
		kind.CellLocal, kind.CellLocal, kind.CellLocal,
		kind.CellFormal, kind.CellFormal, kind.CellFunctionVararg,
		kind.CellCapture, kind.CellCapture,
		kind.CellLoop, kind.CellLoop,
	}
	wantSlots := []uint32{1, 2, 3, 1, 2, 0, 1, 2, 1, 2}
	for index, cell := range terms {
		role, roleOK := result.Role(cell)
		if !roleOK || role != wantRoles[index] {
			t.Fatalf("Role(%v) = %v/%v, want %v", cell, role, roleOK, wantRoles[index])
		}
		slot, slotOK := result.Slot(cell)
		if !slotOK || slot != wantSlots[index] {
			t.Fatalf("Slot(%v) = %d/%v, want %d", cell, slot, slotOK, wantSlots[index])
		}
	}
	if slot, ok := result.Slot(keyspace.MakeTerm(keyspace.FamilyCell, uint32(len(terms)+1))); ok || slot != 0 {
		t.Fatalf("Slot of a Cell outside the denominator = %d/%v, want 0/false", slot, ok)
	}
	if slot, ok := (Result{}).Slot(terms[0]); ok || slot != 0 {
		t.Fatalf("Slot on an unsealed Result = %d/%v, want 0/false", slot, ok)
	}
}
