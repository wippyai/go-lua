package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/position"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
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
	if got, ok := fixture.result.Read(read); !ok || got != function {
		t.Fatalf("ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.Call(call); !ok || got != function {
		t.Fatalf("CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionSoleAssignInstallation(t *testing.T) {
	fixture := openDirectFixture(t, soleAssignSpec())
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.Read(read); !ok || got != function {
		t.Fatalf("sole Assign ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.Call(call); !ok || got != function {
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
	if got, ok := fixture.result.GenericLoop(loop); !ok || got != function {
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
	if got, ok := fixture.result.Read(read); !ok || got != function {
		t.Fatalf("assignment-recursive ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := fixture.result.Call(call); !ok || got != function {
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
	if got, ok := fixture.result.Read(read); ok || got != 0 {
		t.Fatalf("nonrecursive Bind initializer ReadFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := fixture.result.Call(call); ok || got != 0 {
		t.Fatalf("nonrecursive Bind initializer CallFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestDirectFunctionActiveAssignSurvivesStaleBindClaim(t *testing.T) {
	fixture := openDirectFixture(t, staleBindAssignSpec())
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.Call(call); !ok || got != function {
		t.Fatalf("active Assign after stale Bind CallFunction = %v/%v, want %v/true", got, ok, function)
	}
}

func TestDirectFunctionDeadBindAndAssignDoNotInstall(t *testing.T) {
	fixture := openDirectFixture(t, deadBindAssignSpec())
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := fixture.result.Call(call); ok || got != 0 {
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
	if got, ok := fixture.result.Call(call); !ok || got != function {
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
	if got, ok := fixture.result.Call(call); !ok || got != function {
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
	if got, ok := fixture.result.Call(call); ok || got != 0 {
		t.Fatalf("reassigned CallFunction = %v/%v, want 0/false", got, ok)
	}
}

type directFixture struct {
	source     source.View
	flow       authored.View
	bodies     *body.Result
	bindings   binding.Result
	forest     *containment.Result
	control    *sourcecontrol.Result
	executable *executable.Result
	result     *Result

	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type directSpec struct {
	sourceName string
	counts     [keyspace.FamilyCount]uint32
	rows       [][]keyspace.Term
	flow       authored.Input
	binds      []source.BindCells
	forms      []source.FunctionFormals
	nilOwners  []keyspace.Term
}

func openDirectFixture(t *testing.T, spec directSpec) *directFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 || len(spec.rows) != int(spec.counts[keyspace.FamilyBody]) {
		t.Fatal("direct fixture requires one Source row per Body")
	}

	sourceDraft, err := source.Build(directSourceInput(spec))
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{}
	staticInput.Counts[keyspace.FamilyBody] = spec.counts[keyspace.FamilyBody]
	staticInput.Counts[keyspace.FamilyCell] = spec.counts[keyspace.FamilyCell]
	staticInput.Counts[keyspace.FamilyValues] = spec.counts[keyspace.FamilyValues]
	staticInput.Counts[keyspace.FamilyValueClaim] = spec.counts[keyspace.FamilyValueClaim]
	staticInput.Counts[keyspace.FamilyTypePrimitive] = uint32(len(staticInput.Types.Primitive))
	staticInput.Counts[keyspace.FamilyTypeAlias] = uint32(len(staticInput.Declarations.Alias))
	if spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	if spec.counts[keyspace.FamilyCall] != 0 {
		staticInput.Contracts.Call = make([]staticcontracts.CallContract, spec.counts[keyspace.FamilyCall])
	}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticInput.Counts[keyspace.FamilyCall] = uint32(len(staticInput.Contracts.Call))
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}

	flowInput := spec.flow
	flowInput.Counts = spec.counts
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		_ = sourceFinalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	bodies, err := body.Seal(preimage, flowView, staticFinalize.View(), entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("binding.Seal: %v", err)
	}

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, imports.Finalizer{})
		t.Fatalf("imports.Finalizer: %v", err)
	}
	forest, _, err := containment.Prove(preimage, staticFinalize.View(), flowView, bodies, bindingResult, moduleFinalize.View(), entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinalize.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	paths, err := semanticpath.Seal(issuance, sourceView.CellRoles(), sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticFinalize.View().ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("semanticpath.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, forest, controlResult,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID(), paths)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("executable.Seal: %v", err)
	}
	result, err := Seal(
		sourceView, flowView, bodies, bindingResult, forest, controlResult, executableResult,
		staticFinalize.View().ContentID(), moduleFinalize.View().ContentID(),
	)
	if err != nil {
		flowtest.CloseFinalizers(source.Finalizer{}, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("directfunction.Seal: %v", err)
	}

	fixture := &directFixture{
		source: sourceView, flow: flowView, bodies: bodies, bindings: bindingResult,
		forest: forest, control: controlResult, executable: executableResult,
		result: result, staticFinalize: staticFinalize, flowFinalize: flowFinalize,
		moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(source.Finalizer{}, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func directSourceInput(spec directSpec) source.Input {
	name := spec.sourceName
	if name == "" {
		name = "directfunction-law.lua"
	}
	input := source.Input{Name: name}
	input.Families = flowtest.FamilySpans(input.Name, spec.counts)
	input.Bodies = make([]source.BodySource, len(spec.rows))
	for index, rows := range spec.rows {
		input.Bodies[index] = source.BodySource{
			Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: append([]keyspace.Term(nil), rows...),
		}
	}
	input.Binds = make([]source.BindCells, spec.counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		if index < len(spec.binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), spec.binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, spec.counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		if index < len(spec.forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), spec.forms[index].Formals...)
		}
	}
	input.Nil = flowtest.LiteralRows(spec.counts[keyspace.FamilyNil], spec.nilOwners, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	input.Bool = flowtest.LiteralRows(spec.counts[keyspace.FamilyBool], nil, keyspace.MakeTerm(keyspace.FamilyBody, 1), func(owner keyspace.Term, ordinal uint32) source.BoolLiteral {
		return source.BoolLiteral{Owner: owner, Value: ordinal%2 == 1}
	})
	return input
}
