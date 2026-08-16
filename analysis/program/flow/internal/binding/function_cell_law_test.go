package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestFunctionCellUsesPositionalBindValueEvidence(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function1 := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	function2 := keyspace.MakeTerm(keyspace.FamilyFunction, 2)

	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyBind: 2,
		keyspace.FamilyValues: 2, keyspace.FamilyFunction: 2,
	}}
	input.Values.Rows = []authored.Value{
		{Owner: body1, Fixed: authored.Range{Start: 0, End: 1}},
		{Owner: body1, Fixed: authored.Range{Start: 1, End: 2}},
	}
	input.Values.Terms = []keyspace.Term{function1, function2}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body1},
		{Kind: authored.CellLocal, Body: body1},
	}
	input.Storage.Binds = []authored.Bind{
		{Owner: body1, Values: values1},
		{Owner: body1, Values: values2},
	}
	input.Functions.Rows = []authored.Function{
		{Owner: body1, Body: body2},
		{Owner: body1, Body: body3},
	}

	result, finish := sealLawFixture(t, input,
		[][]keyspace.Term{{bind1, bind2}, nil, nil},
		[]source.BindCells{
			{Bind: bind1, Cells: []keyspace.Term{cell2}},
			{Bind: bind2, Cells: []keyspace.Term{cell1}},
		},
		[]source.FunctionFormals{{Function: function1}, {Function: function2}}, nil)
	defer finish()

	if got, ok := result.FunctionCell(function1); !ok || got != cell2 {
		t.Fatalf("FunctionCell(%v) = %v/%v, want %v/true", function1, got, ok, cell2)
	}
	if got, ok := result.FunctionCell(function2); !ok || got != cell1 {
		t.Fatalf("FunctionCell(%v) = %v/%v, want %v/true", function2, got, ok, cell1)
	}
}

func TestFunctionCellFailsClosedForZeroAndNonSelfEvidence(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	base := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyBind: 1,
		keyspace.FamilyValues: 1, keyspace.FamilyFunction: 1,
	}}
	base.Values.Rows = []authored.Value{{Owner: body2, Fixed: authored.Range{End: 1}}}
	base.Values.Terms = []keyspace.Term{function}
	base.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	base.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	base.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}

	result, finish := sealLawFixture(t, base, [][]keyspace.Term{{bind}, nil},
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		[]source.FunctionFormals{{Function: function}}, nil)
	defer finish()
	if got, ok := result.FunctionCell(function); ok || got != 0 {
		t.Fatalf("non-self FunctionCell = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.FunctionCell(0); ok || got != 0 {
		t.Fatalf("zero FunctionCell = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.FunctionCell(keyspace.MakeTerm(keyspace.FamilyCell, 1)); ok || got != 0 {
		t.Fatalf("foreign FunctionCell = %v/%v, want 0/false", got, ok)
	}
}

func TestSealRejectsDuplicateFunctionCellClaims(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyBind: 1,
		keyspace.FamilyValues: 1, keyspace.FamilyFunction: 1,
	}}
	input.Values.Rows = []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}}
	input.Values.Terms = []keyspace.Term{function, function}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body1},
		{Kind: authored.CellLocal, Body: body1},
	}
	input.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}
	_, err, finish := trySealLawFixture(t, input, [][]keyspace.Term{{bind}, nil},
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell1, cell2}}},
		[]source.FunctionFormals{{Function: function}}, nil)
	finish()
	if err == nil {
		t.Fatal("duplicate Function Cell claim was accepted")
	}
}

func TestFunctionCellQueryAllocatesNothing(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	result := Result{
		sourceID:      bindingTestSourceID(),
		flowID:        bindingTestFlowID(),
		roles:         []kind.CellRole{0, kind.CellLocal},
		hosts:         []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyBind, 1)},
		functionCells: []keyspace.Term{0, cell},
	}
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = result.FunctionCell(function)
	})
	if allocs != 0 {
		t.Fatalf("FunctionCell query allocates %.2f times", allocs)
	}
}
