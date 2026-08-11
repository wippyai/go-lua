package control

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestShapeSemanticAcceptsNestedReturn(t *testing.T) {
	counts := controlCounts(3, 1, 2, 0, 0, 0, 0, 0, 0, 0)
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyReturn] = 1
	root := terms(counts, keyspace.FamilyBody, 1)
	child := terms(counts, keyspace.FamilyBody, 2)
	sibling := terms(counts, keyspace.FamilyBody, 3)
	branch := terms(counts, keyspace.FamilyBranch, 1)
	returned := terms(counts, keyspace.FamilyReturn, 1)
	values := terms(counts, keyspace.FamilyValues, 1)
	nilRoot := terms(counts, keyspace.FamilyNil, 1)
	nilChild := terms(counts, keyspace.FamilyNil, 2)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: child, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilChild},
		},
		Control: authored.ControlInput{
			Returns:  []authored.Return{{Owner: child, Values: values}},
			Branches: []authored.Branch{{Owner: root, Condition: nilRoot, WhenTrue: child, WhenFalse: sibling}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{branch}, []keyspace.Term{returned}, nil)
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: rows, flow: input,
		nilOwners: []keyspace.Term{root, child},
	})
	_ = f.seal(t)
}

func TestShapeSemanticAcceptsRepeatBindGotoTerminalLabel(t *testing.T) {
	counts := controlCounts(2, 1, 2, 1, 1, 1, 1, 1, 0, 0)
	root := terms(counts, keyspace.FamilyBody, 1)
	repeatBody := terms(counts, keyspace.FamilyBody, 2)
	loop := terms(counts, keyspace.FamilyLoop, 1)
	bind := terms(counts, keyspace.FamilyBind, 1)
	cell := terms(counts, keyspace.FamilyCell, 1)
	values := terms(counts, keyspace.FamilyValues, 1)
	nilLoop := terms(counts, keyspace.FamilyNil, 1)
	nilBind := terms(counts, keyspace.FamilyNil, 2)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: repeatBody, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilBind},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: repeatBody}},
			Binds: []authored.Bind{{Owner: repeatBody, Values: values}},
		},
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: repeatBody}},
			Gotos:  []authored.Goto{{Owner: repeatBody, Target: label}},
			Loops:  []authored.Loop{{Owner: root, Body: repeatBody, Kind: kind.LoopRepeat, Control: nilLoop}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{loop}, []keyspace.Term{bind, jump, label})
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: rows, flow: input,
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{repeatBody, repeatBody},
	})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != repeatBody {
		t.Fatalf("GotoTargetBody(repeat terminal) = %v/%v, want %v", got, ok, repeatBody)
	}
}

func TestShapeSemanticAcceptsNumericAndGenericForCells(t *testing.T) {
	counts := controlCounts(3, 2, 3, 3, 0, 2, 0, 0, 1, 0)
	root := terms(counts, keyspace.FamilyBody, 1)
	numericBody := terms(counts, keyspace.FamilyBody, 2)
	genericBody := terms(counts, keyspace.FamilyBody, 3)
	numericLoop := terms(counts, keyspace.FamilyLoop, 1)
	genericLoop := terms(counts, keyspace.FamilyLoop, 2)
	numericValues := terms(counts, keyspace.FamilyValues, 1)
	genericValues := terms(counts, keyspace.FamilyValues, 2)
	breakTerm := terms(counts, keyspace.FamilyBreak, 1)
	cell1 := terms(counts, keyspace.FamilyCell, 1)
	cell2 := terms(counts, keyspace.FamilyCell, 2)
	cell3 := terms(counts, keyspace.FamilyCell, 3)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: root, Fixed: authored.Range{Start: 0, End: 2}},
				{Owner: root, Fixed: authored.Range{Start: 2, End: 3}},
			},
			Terms: []keyspace.Term{
				terms(counts, keyspace.FamilyNil, 1),
				terms(counts, keyspace.FamilyNil, 2),
				terms(counts, keyspace.FamilyNil, 3),
			},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: numericBody},
			{Kind: authored.CellLocal, Body: genericBody},
			{Kind: authored.CellLocal, Body: genericBody},
		}},
		Control: authored.ControlInput{
			Breaks: []authored.Break{{Owner: genericBody}},
			Loops: []authored.Loop{
				{Owner: root, Body: numericBody, Kind: kind.LoopNumericFor, Control: numericValues, Cells: authored.Range{Start: 0, End: 1}},
				{Owner: root, Body: genericBody, Kind: kind.LoopGenericFor, Control: genericValues, Cells: authored.Range{Start: 1, End: 3}},
			},
			Cells: []keyspace.Term{cell1, cell2, cell3},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{numericLoop, genericLoop}, nil, []keyspace.Term{breakTerm})
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: rows, flow: input,
		nilOwners: []keyspace.Term{root, root, root},
	})
	shape := f.seal(t)
	if got, ok := shape.BreakLoop(breakTerm); !ok || got != genericLoop {
		t.Fatalf("BreakLoop(generic body) = %v/%v, want %v", got, ok, genericLoop)
	}
}

func TestShapeSemanticRejectsOuterToChildGoto(t *testing.T) {
	counts := controlCounts(3, 0, 1, 0, 0, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyBranch] = 1
	root := terms(counts, keyspace.FamilyBody, 1)
	child := terms(counts, keyspace.FamilyBody, 2)
	sibling := terms(counts, keyspace.FamilyBody, 3)
	branch := terms(counts, keyspace.FamilyBranch, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)
	nilRoot := terms(counts, keyspace.FamilyNil, 1)

	input := authored.Input{
		Control: authored.ControlInput{
			Labels:   []authored.Label{{Owner: child}},
			Gotos:    []authored.Goto{{Owner: root, Target: label}},
			Branches: []authored.Branch{{Owner: root, Condition: nilRoot, WhenTrue: child, WhenFalse: sibling}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{branch, jump}, []keyspace.Term{label}, nil)
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input, nilOwners: []keyspace.Term{root}})
	if err := f.sealError(); err == nil {
		t.Fatal("outer-to-child Goto was accepted")
	}
}

func TestShapeSemanticRejectsSiblingToSiblingGoto(t *testing.T) {
	counts := controlCounts(3, 0, 1, 0, 0, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyBranch] = 1
	root := terms(counts, keyspace.FamilyBody, 1)
	left := terms(counts, keyspace.FamilyBody, 2)
	right := terms(counts, keyspace.FamilyBody, 3)
	branch := terms(counts, keyspace.FamilyBranch, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)
	nilRoot := terms(counts, keyspace.FamilyNil, 1)

	input := authored.Input{
		Control: authored.ControlInput{
			Labels:   []authored.Label{{Owner: right}},
			Gotos:    []authored.Goto{{Owner: left, Target: label}},
			Branches: []authored.Branch{{Owner: root, Condition: nilRoot, WhenTrue: left, WhenFalse: right}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{branch}, []keyspace.Term{jump}, []keyspace.Term{label})
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input, nilOwners: []keyspace.Term{root}})
	if err := f.sealError(); err == nil {
		t.Fatal("sibling-to-sibling Goto was accepted")
	}
}

func TestShapeSemanticAcceptsBoundChildOutwardGoto(t *testing.T) {
	counts := controlCounts(3, 1, 2, 1, 1, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyBranch] = 1
	root := terms(counts, keyspace.FamilyBody, 1)
	child := terms(counts, keyspace.FamilyBody, 2)
	sibling := terms(counts, keyspace.FamilyBody, 3)
	branch := terms(counts, keyspace.FamilyBranch, 1)
	bind := terms(counts, keyspace.FamilyBind, 1)
	cell := terms(counts, keyspace.FamilyCell, 1)
	values := terms(counts, keyspace.FamilyValues, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)
	nilRoot := terms(counts, keyspace.FamilyNil, 1)
	nilChild := terms(counts, keyspace.FamilyNil, 2)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: child, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilChild},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}},
			Binds: []authored.Bind{{Owner: child, Values: values}},
		},
		Control: authored.ControlInput{
			Labels:   []authored.Label{{Owner: root}},
			Gotos:    []authored.Goto{{Owner: child, Target: label}},
			Branches: []authored.Branch{{Owner: root, Condition: nilRoot, WhenTrue: child, WhenFalse: sibling}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{label, branch}, []keyspace.Term{bind, jump}, nil)
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: rows, flow: input,
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{root, child},
	})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != root {
		t.Fatalf("GotoTargetBody(bound child outward) = %v/%v, want %v", got, ok, root)
	}
}

func TestShapeSemanticAcceptsInteriorLabelAfterBindBackwardGoto(t *testing.T) {
	counts := controlCounts(1, 1, 1, 1, 1, 0, 1, 1, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	bind := terms(counts, keyspace.FamilyBind, 1)
	cell := terms(counts, keyspace.FamilyCell, 1)
	values := terms(counts, keyspace.FamilyValues, 1)
	nilValue := terms(counts, keyspace.FamilyNil, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilValue},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body}},
			Gotos:  []authored.Goto{{Owner: body, Target: label}},
		},
		Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: bodyRows([]keyspace.Term{bind, label, jump}), flow: input,
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{body},
	})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != body {
		t.Fatalf("GotoTargetBody(interior backward) = %v/%v, want %v", got, ok, body)
	}
}

func TestShapeSemanticRejectsOuterToFunctionBodyLabel(t *testing.T) {
	counts := controlCounts(2, 1, 0, 0, 0, 0, 1, 1, 0, 1)
	counts[keyspace.FamilyReturn] = 1
	root := terms(counts, keyspace.FamilyBody, 1)
	functionBody := terms(counts, keyspace.FamilyBody, 2)
	function := terms(counts, keyspace.FamilyFunction, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	jump := terms(counts, keyspace.FamilyGoto, 1)
	returned := terms(counts, keyspace.FamilyReturn, 1)

	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: root, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{function},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: root, Body: functionBody}}},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: root, Values: terms(counts, keyspace.FamilyValues, 1)}},
			Labels:  []authored.Label{{Owner: functionBody}},
			Gotos:   []authored.Goto{{Owner: root, Target: label}},
		},
		Counts: counts,
	}
	rows := bodyRows([]keyspace.Term{returned, jump}, []keyspace.Term{label})
	f := openShapeFixture(t, shapeSpec{
		counts: counts, rows: rows, flow: input,
		forms: []source.FunctionFormals{{Function: function}},
	})
	if err := f.sealError(); err == nil {
		t.Fatal("outer-to-function-body Goto was accepted")
	}
}
