package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestDirectFunctionRetainsDominatedReadAndCall(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)

	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1,
		keyspace.FamilyCall: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, call}, {}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: assignValues}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Calls:     []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
		},
	})
	if got, ok := fixture.result.ReadFunction(read); !ok || got != function {
		t.Fatalf("ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionSoleAssignInstallation(t *testing.T) {
	fixture := openDirectFixture(t, soleAssignSpec())
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.ReadFunction(read); !ok || got != function {
		t.Fatalf("sole Assign ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("sole Assign CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func soleAssignSpec() directSpec {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 1,
			keyspace.FamilyValues: 3, keyspace.FamilyBind: 1, keyspace.FamilyAssign: 1,
			keyspace.FamilyWrite: 1, keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1,
			keyspace.FamilyCall: 1,
		},
		rows:  [][]keyspace.Term{{bind, assign, call}, {}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms: []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), function},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads:   []authored.Read{{Owner: body1, Source: cell}},
				Binds:   []authored.Bind{{Owner: body1, Values: bindValues}},
				Assigns: []authored.Assign{{Owner: body1, Values: assignValues}},
				Writes:  []authored.Write{{Assign: assign, Target: cell}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Calls:     []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
		},
	}
}

func TestDirectFunctionRetainsGenericForFunction(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	bindCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	loopCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	controlValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1, keyspace.FamilyLoop: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, loop}, {}, {}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{bindCell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{function, read},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body3}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}},
				Reads: []authored.Read{{Owner: body1, Source: bindCell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Control: authored.ControlInput{
				Loops: []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopGenericFor, Control: controlValues, Cells: authored.Range{End: 1}}},
				Cells: []keyspace.Term{loopCell},
			},
		},
	})
	if got, ok := fixture.result.GenericLoopFunction(loop); !ok || got != function {
		t.Fatalf("GenericLoopFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionRetainsRecursiveSelfCapture(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyValues: 3,
		keyspace.FamilyBind: 1, keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 1,
		keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, assign}, {call}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 1}},
					{Owner: body1, Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: body2, Fixed: authored.Range{Start: 2, End: 2}},
				},
				Terms: []keyspace.Term{nilTerm, function},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}},
				Reads:   []authored.Read{{Owner: body2, Source: inner}},
				Binds:   []authored.Bind{{Owner: body1, Values: bindValues}},
				Assigns: []authored.Assign{{Owner: body1, Values: assignValues}},
				Writes:  []authored.Write{{Assign: assign, Target: cell}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: cell}},
			},
			Calls: []authored.Call{{Owner: body2, Callee: read, Actuals: actuals}},
		},
	})
	if got, ok := fixture.result.ReadFunction(read); !ok || got != function {
		t.Fatalf("assignment-recursive ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("assignment-recursive CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionBindInitializerIsNotRecursiveWithoutSelfCapture(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	other := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	otherBind := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	otherValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 3,
		keyspace.FamilyValues: 3, keyspace.FamilyBind: 2, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, otherBind}, {call}},
		binds: []source.BindCells{
			{Bind: bind, Cells: []keyspace.Term{cell}},
			{Bind: otherBind, Cells: []keyspace.Term{other}},
		},
		forms: []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body2, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{function, nilTerm},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body1},
				},
				Reads: []authored.Read{{Owner: body2, Source: inner}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}, {Owner: body1, Values: otherValues}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: other}},
			},
			Calls: []authored.Call{{Owner: body2, Callee: read, Actuals: actuals}},
		},
	})
	if got, ok := fixture.result.ReadFunction(read); ok || got != 0 {
		t.Fatalf("nonrecursive Bind initializer ReadFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := fixture.result.CallFunction(call); ok || got != 0 {
		t.Fatalf("nonrecursive Bind initializer CallFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestDirectFunctionActiveAssignSurvivesStaleBindClaim(t *testing.T) {
	fixture := openDirectFixture(t, staleBindAssignSpec())
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("active Assign after stale Bind CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionDeadBindAndAssignDoNotInstall(t *testing.T) {
	fixture := openDirectFixture(t, deadBindAssignSpec())
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.CallFunction(call); ok || got != 0 {
		t.Fatalf("dead Bind and Assign CallFunction = %v/%v, want 0/false", got, ok)
	}
}

func staleBindAssignSpec() directSpec {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	function1 := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	function2 := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 3, keyspace.FamilyCell: 1, keyspace.FamilyValues: 3,
			keyspace.FamilyBind: 1, keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 1,
			keyspace.FamilyFunction: 2, keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
		},
		rows:  [][]keyspace.Term{{bind, assign, call}, {}, {}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms: []source.FunctionFormals{{Function: function1}, {Function: function2}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 1}},
					{Owner: body1, Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: body1, Fixed: authored.Range{Start: 2, End: 2}},
				},
				Terms: []keyspace.Term{function1, function2},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads:   []authored.Read{{Owner: body1, Source: cell}},
				Binds:   []authored.Bind{{Owner: body1, Values: bindValues}},
				Assigns: []authored.Assign{{Owner: body1, Values: assignValues}},
				Writes:  []authored.Write{{Assign: assign, Target: cell}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{
				{Owner: body1, Body: body2}, {Owner: body1, Body: body3},
			}},
			Calls: []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
		},
	}
}

func deadBindAssignSpec() directSpec {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	returnValues := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	function1 := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	function2 := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return directSpec{
		counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 3, keyspace.FamilyCell: 1, keyspace.FamilyValues: 4,
			keyspace.FamilyBind: 1, keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 1,
			keyspace.FamilyFunction: 2, keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
			keyspace.FamilyReturn: 1,
		},
		rows:  [][]keyspace.Term{{call, returnTerm, bind, assign}, {}, {}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms: []source.FunctionFormals{{Function: function1}, {Function: function2}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 1}},
					{Owner: body1, Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: body1, Fixed: authored.Range{Start: 2, End: 2}},
					{Owner: body1, Fixed: authored.Range{Start: 2, End: 2}},
				},
				Terms: []keyspace.Term{function1, function2},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads:   []authored.Read{{Owner: body1, Source: cell}},
				Binds:   []authored.Bind{{Owner: body1, Values: bindValues}},
				Assigns: []authored.Assign{{Owner: body1, Values: assignValues}},
				Writes:  []authored.Write{{Assign: assign, Target: cell}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{
				{Owner: body1, Body: body2}, {Owner: body1, Body: body3},
			}},
			Calls:   []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body1, Values: returnValues}}},
		},
	}
}

func TestDirectFunctionRetainsBranchDominatedInstallation(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 4, keyspace.FamilyCell: 1,
		keyspace.FamilyValues: 2, keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1, keyspace.FamilyBranch: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, branch}, {call}, {}, {}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads: []authored.Read{{Owner: body2, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body4}}},
			Calls:     []authored.Call{{Owner: body2, Callee: read, Actuals: actuals}},
			Control: authored.ControlInput{Branches: []authored.Branch{{
				Owner: body1, Condition: keyspace.MakeTerm(keyspace.FamilyNil, 1), WhenTrue: body2, WhenFalse: body3,
			}}},
		},
	})
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("branch CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionRetainsGotoDominatedInstallation(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	jump := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 1, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1,
		keyspace.FamilyCall: 1, keyspace.FamilyGoto: 1, keyspace.FamilyLabel: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, jump, label, call}, {}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Calls:     []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
			Control:   authored.ControlInput{Labels: []authored.Label{{Owner: body1}}, Gotos: []authored.Goto{{Owner: body1, Target: label}}},
		},
	})
	if got, ok := fixture.result.CallFunction(call); !ok || got != function {
		t.Fatalf("goto CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionRejectsReassignment(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign1 := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	assign2 := keyspace.MakeTerm(keyspace.FamilyAssign, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTerm2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 2, keyspace.FamilyBody: 2, keyspace.FamilyCell: 1,
		keyspace.FamilyValues: 4, keyspace.FamilyBind: 1, keyspace.FamilyAssign: 2,
		keyspace.FamilyWrite: 2, keyspace.FamilyFunction: 1, keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	fixture := openDirectFixture(t, directSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, assign1, assign2, call}, {}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		forms:  []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 3}}, {Owner: body1, Fixed: authored.Range{Start: 3, End: 3}}},
				Terms: []keyspace.Term{nilTerm, function, nilTerm2},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}},
				Reads:   []authored.Read{{Owner: body1, Source: cell}},
				Binds:   []authored.Bind{{Owner: body1, Values: values1}},
				Assigns: []authored.Assign{{Owner: body1, Values: values2}, {Owner: body1, Values: values3}},
				Writes:  []authored.Write{{Assign: assign1, Target: cell}, {Assign: assign2, Target: cell}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Calls:     []authored.Call{{Owner: body1, Callee: read, Actuals: actuals}},
		},
	})
	if got, ok := fixture.result.CallFunction(call); ok || got != 0 {
		t.Fatalf("reassigned CallFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestSealRejectsSameDenominatorForeignOwners(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 3, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	base := directSpec{
		sourceName: "directfunction-owner-a.lua",
		counts:     counts,
		rows:       [][]keyspace.Term{{bind, call}, {}},
		binds:      []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell, outer}}},
		forms:      []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{function, keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body1},
				},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: outer}},
			},
			Calls: []authored.Call{{Owner: body1, Callee: read, Actuals: callValues}},
		},
	}
	foreign := base
	foreign.sourceName = "directfunction-owner-b.lua"
	foreign.flow.Values.Rows = append([]authored.Value(nil), base.flow.Values.Rows...)
	foreign.flow.Functions.Captures = []authored.Capture{{Inner: inner, Outer: cell}}
	left := openDirectFixture(t, base)
	right := openDirectFixture(t, foreign)
	staticID := left.staticFinalize.View().ContentID()
	moduleID := left.moduleFinalize.View().ContentID()

	if _, err := Seal(left.source, right.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Flow with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, right.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign executable proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, right.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign containment proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, right.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign source-control proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, right.bodies, left.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Body proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, right.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Binding proof with equal denominators was accepted")
	}
	foreignStaticID := staticID
	foreignStaticID[0] ^= 1
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, foreignStaticID, moduleID); err == nil {
		t.Fatal("foreign Static identity with equal denominators was accepted")
	}
	foreignModuleID := moduleID
	foreignModuleID[0] ^= 1
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, staticID, foreignModuleID); err == nil {
		t.Fatal("foreign Module identity with equal denominators was accepted")
	}
}

func TestDirectFunctionProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	baseSpec := directProvenanceSpec()
	base := openDirectFixture(t, baseSpec)

	foreignSourceSpec := baseSpec
	foreignSourceSpec.sourceName = "directfunction-provenance-foreign-source.lua"
	foreignSource := openDirectFixture(t, foreignSourceSpec)

	foreignFlowSpec := baseSpec
	foreignFlowSpec.flow.Values.Rows = append([]authored.Value(nil), baseSpec.flow.Values.Rows...)
	foreignFlowSpec.flow.Functions.Captures = []authored.Capture{{
		Inner: keyspace.MakeTerm(keyspace.FamilyCell, 2),
		Outer: keyspace.MakeTerm(keyspace.FamilyCell, 1),
	}}
	foreignFlow := openDirectFixture(t, foreignFlowSpec)

	sourceID := base.source.Identity().ContentID()
	flowID := base.flow.Cold().ContentID()
	staticID := base.staticFinalize.View().ContentID()
	moduleID := base.moduleFinalize.View().ContentID()
	foreignSourceID := foreignSource.source.Identity().ContentID()
	foreignFlowID := foreignFlow.flow.Cold().ContentID()
	foreignSourceStaticID := foreignSource.staticFinalize.View().ContentID()
	foreignSourceModuleID := foreignSource.moduleFinalize.View().ContentID()
	foreignFlowStaticID := foreignFlow.staticFinalize.View().ContentID()
	foreignFlowModuleID := foreignFlow.moduleFinalize.View().ContentID()
	if !Matches(base.result, sourceID, flowID, staticID, moduleID) ||
		!Matches(foreignSource.result, foreignSourceID, flowID, foreignSourceStaticID, foreignSourceModuleID) ||
		!Matches(foreignFlow.result, sourceID, foreignFlowID, foreignFlowStaticID, foreignFlowModuleID) {
		t.Fatal("direct-function result did not retain exact four owner identities")
	}
	if sourceID == foreignSourceID || flowID == foreignFlowID ||
		base.source.Identity().TermCount() != foreignSource.source.Identity().TermCount() ||
		base.flow.Values().Count() != foreignFlow.flow.Values().Count() {
		t.Fatal("foreign direct-function fixtures did not preserve equal denominators with distinct identities")
	}
	if Matches(base.result, foreignSourceID, flowID, staticID, moduleID) ||
		Matches(foreignSource.result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("direct-function provenance accepted an equal-denominator foreign Source")
	}
	if Matches(base.result, sourceID, foreignFlowID, staticID, moduleID) ||
		Matches(foreignFlow.result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("direct-function provenance accepted an equal-denominator foreign Flow")
	}
	ids := [4]identity.ContentID{sourceID, flowID, staticID, moduleID}
	for index, name := range []string{"Source", "Flow", "Static", "Module"} {
		foreign := ids[index]
		foreign[0] ^= 1
		candidate := ids
		candidate[index] = foreign
		if Matches(base.result, candidate[0], candidate[1], candidate[2], candidate[3]) {
			t.Fatalf("direct-function Matches accepted foreign %s identity", name)
		}
	}
}

func directProvenanceSpec() directSpec {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 3, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	return directSpec{
		sourceName: "directfunction-provenance.lua",
		counts:     counts,
		rows:       [][]keyspace.Term{{bind, call}, {}},
		binds:      []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell, outer}}},
		forms:      []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{function, keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body1},
				},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: outer}},
			},
			Calls: []authored.Call{{Owner: body1, Callee: read, Actuals: callValues}},
		},
	}
}
