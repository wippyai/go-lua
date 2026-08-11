package binding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestSealRejectsUnclassifiedAndDoubleClaimedCells(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil, nil)
	finish()
	if err == nil {
		t.Fatal("unclassified local Cell was accepted")
	}

	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	input.Counts[keyspace.FamilyBind] = 1
	input.Counts[keyspace.FamilyValues] = 1
	input.Counts[keyspace.FamilyVararg] = 1
	input.Values.Rows = []authored.Value{{Owner: body}}
	input.Storage.Binds = []authored.Bind{{Owner: body, Values: values}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body, Cell: cell}}
	_, err, finish = trySealLawFixture(t, input, [][]keyspace.Term{{bind}},
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}, nil, nil)
	finish()
	if err == nil {
		t.Fatal("Cell claimed by Bind and chunk Vararg was accepted")
	}
}

func TestSealRejectsFormalWrongBodyAndDuplicateRole(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyFunction: 1,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cell}}}, nil)
	finish()
	if err == nil {
		t.Fatal("formal Cell with wrong defining Body was accepted")
	}

	input.Storage.Cells[0].Body = body2
	input.Functions.Rows[0].Vararg = cell
	_, err, finish = trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cell}}}, nil)
	finish()
	if err == nil {
		t.Fatal("formal and Function-vararg double claim was accepted")
	}
}

func TestAuthoredRejectsDuplicateFormalAndLoopRows(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	duplicate := source.Input{Name: "duplicate-formal.lua", Bodies: []source.BodySource{
		{Body: body1}, {Body: body2},
	}, Functions: []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cell, cell}}}}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		switch family {
		case keyspace.FamilyBody:
			count = 2
		case keyspace.FamilyCell, keyspace.FamilyFunction:
			count = 1
		}
		spans := make([]source.Span, count)
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: duplicate.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		duplicate.Families = append(duplicate.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	if _, err := source.Build(duplicate); err == nil {
		t.Fatal("duplicate formal order was admitted by Source")
	}

	loopInput := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyLoop: 1, keyspace.FamilyNil: 1,
	}}
	loopInput.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}}
	loopInput.Control.Loops = []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1), Cells: authored.Range{Start: 0, End: 2}}}
	loopInput.Control.Cells = []keyspace.Term{cell, cell}
	if _, err := authored.Build(loopInput); err == nil {
		t.Fatal("duplicate Loop Cells were admitted")
	}
}

func TestAuthoredRejectsLoopAndFunctionVarargWrongBody(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	loopInput := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyLoop: 1, keyspace.FamilyNil: 1,
	}}
	loopInput.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	loopInput.Control.Loops = []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1), Cells: authored.Range{Start: 0, End: 1}}}
	loopInput.Control.Cells = []keyspace.Term{cell}
	if _, err := authored.Build(loopInput); err == nil {
		t.Fatal("Loop Cell with wrong Body was admitted")
	}

	functionInput := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyFunction: 1}}
	functionInput.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	functionInput.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: cell}}
	if _, err := authored.Build(functionInput); err == nil {
		t.Fatal("Function vararg Cell with wrong Body was admitted")
	}
}

func TestSealAcceptsFunctionVarargWithoutOccurrence(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyFunction: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: cell}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function}}, nil)
	defer finish()
	if role, ok := result.Role(cell); !ok || role != kind.CellFunctionVararg {
		t.Fatalf("Role(Function.Vararg) = %v/%v", role, ok)
	}
}
