package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestContinuationSealBindPhaseAndFrontier(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBindPhaseSpec())
	first := continuationTerm(keyspace.FamilyUnary, 1)
	second := continuationTerm(keyspace.FamilyUnary, 2)
	cell := continuationTerm(keyspace.FamilyCell, 1)
	if count, ok := fixture.result.CellCount(first); !ok || count != 0 {
		t.Fatalf("pre-Bind Unary Cells = %d/%v, want 0/true", count, ok)
	}
	if count, ok := fixture.result.CellCount(second); !ok || count != 1 {
		t.Fatalf("post-Bind Unary Cells = %d/%v, want 1/true", count, ok)
	}
	if got, ok := fixture.result.CellAt(second, 0); !ok || got != cell {
		t.Fatalf("post-Bind CellAt(0) = %08x/%v, want %08x/true", uint32(got), ok, uint32(cell))
	}
	if _, ok := fixture.result.CellAt(first, 0); ok {
		t.Fatal("pre-Bind Unary exposed the later Cell")
	}
}

func TestContinuationSealEntryChunkVarargIsTheOuterCell(t *testing.T) {
	fixture := openContinuationFixture(t, continuationEntryVarargSpec())
	call := continuationTerm(keyspace.FamilyCall, 1)
	cell := continuationTerm(keyspace.FamilyCell, 1)
	if count, ok := fixture.result.CellCount(call); !ok || count != 1 {
		t.Fatalf("entry chunk-vararg CellCount = %d/%v, want 1/true", count, ok)
	}
	if got, ok := fixture.result.CellAt(call, 0); !ok || got != cell {
		t.Fatalf("entry chunk-vararg CellAt(0) = %08x/%v, want %08x/true", uint32(got), ok, uint32(cell))
	}
}

func TestContinuationSealRejectsBindingRoleHostTamper(t *testing.T) {
	fixture := openContinuationFixture(t, continuationFunctionCellSpec())
	malformed := binding.Result{}
	if err := validateCells(fixture.flow, malformed, fixture.bodies, continuationFunctionCellSpec().counts, continuationTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("Cell role/host tamper was accepted")
	}
	if _, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, malformed, fixture.executable, fixture.candidates, fixture.causal, fixture.staticID, fixture.moduleID); err == nil {
		t.Fatal("Seal accepted a malformed Binding role/host projection")
	}
}

func continuationEntryVarargSpec() continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	call := continuationTerm(keyspace.FamilyCall, 1)
	cell := continuationTerm(keyspace.FamilyCell, 1)
	vararg := continuationTerm(keyspace.FamilyVararg, 1)
	values := continuationTerm(keyspace.FamilyValues, 1)
	nilValue := continuationTerm(keyspace.FamilyNil, 1)
	return continuationSpec{
		name: "continuation-entry-vararg.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyCell, 1),
			familyCount(keyspace.FamilyVararg, 1), familyCount(keyspace.FamilyCall, 1),
			familyCount(keyspace.FamilyValues, 1), familyCount(keyspace.FamilyNil, 1),
		),
		rows:      [][]keyspace.Term{{call}},
		nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{vararg}},
			Calls:   []authored.Call{{Owner: body, Callee: nilValue, Actuals: values}},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Varargs: []authored.Vararg{{Owner: body, Cell: cell}}},
		},
	}
}

func continuationBindPhaseSpec() continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	bind := continuationTerm(keyspace.FamilyBind, 1)
	cell := continuationTerm(keyspace.FamilyCell, 1)
	call := continuationTerm(keyspace.FamilyCall, 1)
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2), continuationTerm(keyspace.FamilyValues, 3)}
	return continuationSpec{
		name: "continuation-bind-phase.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyBind, 1),
			familyCount(keyspace.FamilyCell, 1), familyCount(keyspace.FamilyValues, 3), familyCount(keyspace.FamilyCall, 1), familyCount(keyspace.FamilyReturn, 1),
			familyCount(keyspace.FamilyUnary, 2), familyCount(keyspace.FamilyNil, 3),
		),
		rows:      [][]keyspace.Term{{call, bind, continuationTerm(keyspace.FamilyReturn, 1)}},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{body, body, body},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 1), continuationTerm(keyspace.FamilyUnary, 2)}},
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values[1]}}},
			Calls:     []authored.Call{{Owner: body, Callee: continuationTerm(keyspace.FamilyNil, 3), Actuals: values[0]}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: continuationTerm(keyspace.FamilyNil, 1)}, {Owner: body, Op: kind.UnaryNeg, Operand: continuationTerm(keyspace.FamilyNil, 2)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values[2]}}},
		},
	}
}

func TestContinuationSealFunctionIsolationFormalsVarargAndCapture(t *testing.T) {
	fixture := openContinuationFixture(t, continuationFunctionCellSpec())
	call := continuationTerm(keyspace.FamilyUnary, 1)
	want := []keyspace.Term{
		continuationTerm(keyspace.FamilyCell, 2),
		continuationTerm(keyspace.FamilyCell, 4),
		continuationTerm(keyspace.FamilyCell, 3),
	}
	if count, ok := fixture.result.CellCount(call); !ok || count != len(want) {
		t.Fatalf("Function body Cells = %d/%v, want %d/true", count, ok, len(want))
	}
	for index, wantCell := range want {
		if got, ok := fixture.result.CellAt(call, index); !ok || got != wantCell {
			t.Fatalf("Function CellAt(%d) = %08x/%v, want %08x/true", index, uint32(got), ok, uint32(wantCell))
		}
	}
	outer := continuationTerm(keyspace.FamilyCell, 1)
	for index := 0; index < len(want); index++ {
		if got, ok := fixture.result.CellAt(call, index); ok && got == outer {
			t.Fatal("Function lexical root leaked captured outer Cell")
		}
	}
}

func continuationFunctionCellSpec() continuationSpec {
	body, child := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2)
	function := continuationTerm(keyspace.FamilyFunction, 1)
	bind := continuationTerm(keyspace.FamilyBind, 1)
	cells := []keyspace.Term{continuationTerm(keyspace.FamilyCell, 1), continuationTerm(keyspace.FamilyCell, 2), continuationTerm(keyspace.FamilyCell, 3), continuationTerm(keyspace.FamilyCell, 4)}
	nilValue := continuationTerm(keyspace.FamilyNil, 1)
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	return continuationSpec{
		name: "continuation-function-cells.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 2), familyCount(keyspace.FamilyCell, 4), familyCount(keyspace.FamilyBind, 1),
			familyCount(keyspace.FamilyFunction, 1), familyCount(keyspace.FamilyVararg, 1), familyCount(keyspace.FamilyValues, 2),
			familyCount(keyspace.FamilyNil, 1), familyCount(keyspace.FamilyUnary, 1), familyCount(keyspace.FamilyReturn, 1),
		),
		rows:      [][]keyspace.Term{{bind}, {continuationTerm(keyspace.FamilyReturn, 1)}},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cells[0]}}},
		forms:     []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cells[1]}}},
		nilOwners: []keyspace.Term{child},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: child, Fixed: authored.Range{Start: 1, End: 3}}}, Terms: []keyspace.Term{function, continuationTerm(keyspace.FamilyUnary, 1), continuationTerm(keyspace.FamilyVararg, 1)}},
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: child}, {Kind: authored.CellLocal, Body: child}, {Kind: authored.CellLocal, Body: child}}, Varargs: []authored.Vararg{{Owner: child, Cell: cells[3]}}, Binds: []authored.Bind{{Owner: body, Values: values[0]}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: child, Vararg: cells[3], Captures: authored.Range{End: 1}}}, Captures: []authored.Capture{{Inner: cells[2], Outer: cells[0]}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: child, Op: kind.UnaryNeg, Operand: nilValue}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: child, Values: values[1]}}},
		},
	}
}

func TestContinuationSealLoopChildScopeOnly(t *testing.T) {
	fixture := openContinuationFixture(t, continuationLoopCellSpec())
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	childUnary := continuationTerm(keyspace.FamilyUnary, 1)
	loopCell := continuationTerm(keyspace.FamilyCell, 1)
	if count, ok := fixture.result.CellCount(loop); !ok || count != 0 {
		t.Fatalf("Loop header Cells = %d/%v, want 0/true", count, ok)
	}
	if count, ok := fixture.result.CellCount(childUnary); !ok || count != 1 {
		t.Fatalf("Loop child Cells = %d/%v, want 1/true", count, ok)
	}
	if got, ok := fixture.result.CellAt(childUnary, 0); !ok || got != loopCell {
		t.Fatalf("Loop child CellAt(0) = %08x/%v, want %08x/true", uint32(got), ok, uint32(loopCell))
	}
}

func TestContinuationSealRepeatConditionUsesChildTailFrontier(t *testing.T) {
	fixture := openContinuationFixture(t, continuationRepeatFrontierSpec())
	cell := continuationTerm(keyspace.FamilyCell, 1)
	for _, subject := range []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 1), continuationTerm(keyspace.FamilyUnary, 2)} {
		count, ok := fixture.result.CellCount(subject)
		if !ok || count != 1 {
			t.Fatalf("Repeat frontier subject %08x Cells = %d/%v, want 1/true", uint32(subject), count, ok)
		}
		got, gotOK := fixture.result.CellAt(subject, 0)
		if !gotOK || got != cell {
			t.Fatalf("Repeat frontier subject %08x CellAt(0) = %08x/%v, want %08x/true", uint32(subject), uint32(got), gotOK, uint32(cell))
		}
	}
}

func continuationRepeatFrontierSpec() continuationSpec {
	body, child := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2)
	bind, loop := continuationTerm(keyspace.FamilyBind, 1), continuationTerm(keyspace.FamilyLoop, 1)
	cell := continuationTerm(keyspace.FamilyCell, 1)
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	nils := []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyNil, 2)}
	return continuationSpec{
		name: "continuation-repeat-frontier.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 2), familyCount(keyspace.FamilyBind, 1), familyCount(keyspace.FamilyCell, 1),
			familyCount(keyspace.FamilyLoop, 1), familyCount(keyspace.FamilyReturn, 1), familyCount(keyspace.FamilyValues, 2),
			familyCount(keyspace.FamilyUnary, 2), familyCount(keyspace.FamilyNil, 2),
		),
		rows:      [][]keyspace.Term{{bind, loop}, {continuationTerm(keyspace.FamilyReturn, 1)}},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{child, child},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: child, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 2)}},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values[0]}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{
				{Owner: child, Op: kind.UnaryNeg, Operand: nils[0]},
				{Owner: child, Op: kind.UnaryNeg, Operand: nils[1]},
			}},
			Control: authored.ControlInput{
				Loops:   []authored.Loop{{Owner: body, Body: child, Kind: kind.LoopRepeat, Control: continuationTerm(keyspace.FamilyUnary, 1)}},
				Returns: []authored.Return{{Owner: child, Values: values[1]}},
			},
		},
	}
}

func continuationLoopCellSpec() continuationSpec {
	body, child := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2)
	loop, values := continuationTerm(keyspace.FamilyLoop, 1), []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	return continuationSpec{
		name:   "continuation-loop-cells.lua",
		counts: testContinuationCounts(familyCount(keyspace.FamilyBody, 2), familyCount(keyspace.FamilyLoop, 1), familyCount(keyspace.FamilyCell, 1), familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 2), familyCount(keyspace.FamilyUnary, 1), familyCount(keyspace.FamilyReturn, 1)),
		rows:   [][]keyspace.Term{{loop}, {continuationTerm(keyspace.FamilyReturn, 1)}}, nilOwners: []keyspace.Term{body, child},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: child, Fixed: authored.Range{Start: 1, End: 2}}}, Terms: []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyUnary, 1)}},
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: child, Op: kind.UnaryNeg, Operand: continuationTerm(keyspace.FamilyNil, 2)}}},
			Control:   authored.ControlInput{Loops: []authored.Loop{{Owner: body, Body: child, Kind: kind.LoopGenericFor, Control: values[0], Cells: authored.Range{End: 1}}}, Returns: []authored.Return{{Owner: child, Values: values[1]}}, Cells: []keyspace.Term{continuationTerm(keyspace.FamilyCell, 1)}},
		},
	}
}
