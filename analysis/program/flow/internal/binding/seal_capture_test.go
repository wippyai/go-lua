package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestAuthoredRejectsCaptureInnerWrongBodyAndGlobalOrSameBodyOuter(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	base := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1}}
	base.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{Start: 0, End: 1}}}

	wrongInner := base
	wrongInner.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body1}}
	wrongInner.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	if _, err := authored.Build(wrongInner); err == nil {
		t.Fatal("Capture Inner with wrong Function Body was admitted")
	}

	globalOuter := base
	globalOuter.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellGlobal, Key: 1}}
	globalOuter.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	if _, err := authored.Build(globalOuter); err == nil {
		t.Fatal("global Capture Outer was admitted")
	}

	sameBodyOuter := base
	sameBodyOuter.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body2}}
	sameBodyOuter.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	if _, err := authored.Build(sameBodyOuter); err == nil {
		t.Fatal("same-Function-Body Capture Outer was admitted")
	}
}

func TestSealRejectsCaptureInnerDoubleRole(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body1}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{Start: 0, End: 1}}}
	input.Functions.Captures = []authored.Capture{{Inner: cell, Outer: outer}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cell}}}, nil)
	finish()
	if err == nil {
		t.Fatal("double-claimed Capture Inner was accepted")
	}
}

func TestSealRejectsCaptureOuterNonAncestorAndDuplicateOuter(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1,
		keyspace.FamilyLoop: 1, keyspace.FamilyNil: 1,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body3}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{Start: 0, End: 1}}}
	input.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	input.Control.Loops = []authored.Loop{{Owner: body1, Body: body3, Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)}}
	rows := [][]keyspace.Term{{loop}, {}, {}}
	_, err, finish := trySealLawFixture(t, input, rows, nil,
		[]source.FunctionFormals{{Function: function}}, nil)
	finish()
	if err == nil {
		t.Fatal("non-ancestor Capture Outer was accepted")
	}

	duplicate := input
	duplicate.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}, {Inner: inner, Outer: outer}}
	duplicate.Functions.Rows[0].Captures.End = 2
	duplicate.Counts[keyspace.FamilyCell] = 2
	if _, err := authored.Build(duplicate); err == nil {
		t.Fatal("duplicate Capture Outer within Function was admitted")
	}
}

func TestSealAcceptsCaptureOuterAncestorOrSelf(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1, keyspace.FamilyBind: 1, keyspace.FamilyValues: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body1}}
	input.Values.Rows = []authored.Value{{Owner: body1}}
	input.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{Start: 0, End: 1}}}
	input.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{bind}, {}}, []source.BindCells{{Bind: bind, Cells: []keyspace.Term{outer}}},
		[]source.FunctionFormals{{Function: function}}, nil)
	defer finish()
	if role, ok := result.Role(inner); !ok || role != kind.CellCapture {
		t.Fatalf("Role(Capture.Inner) = %v/%v", role, ok)
	}
}

func TestSealRejectsCaptureAcrossFunctionActivation(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	function1 := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	function2 := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyValues: 1,
	}}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body3},
		{Kind: authored.CellLocal, Body: body1},
	}
	input.Values.Rows = []authored.Value{{Owner: body1}}
	input.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	input.Functions.Rows = []authored.Function{
		{Owner: body1, Body: body2},
		{Owner: body2, Body: body3, Captures: authored.Range{Start: 0, End: 1}},
	}
	input.Functions.Captures = []authored.Capture{{Inner: inner, Outer: outer}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{bind}, {}, {}},
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{outer}}},
		[]source.FunctionFormals{{Function: function1}, {Function: function2}}, nil)
	finish()
	if err == nil {
		t.Fatal("Capture Outer across an intervening Function activation was accepted")
	}
}
