package control

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

// shapeFixture deliberately keeps every owner capability alive. Shape is a
// derived control projection, so its input is assembled through the same
// Source, Static, Flow, Body, Binding, and Containment seals used by the
// eventual top-level Flow finalizer. It never constructs an owner Result.
type shapeFixture struct {
	preimage source.Preimage
	flow     authored.View
	bodies   *body.Result
	binding  binding.Result
	forest   *containment.Result

	sourceFinalize source.Finalizer
	staticFinalize static.Finalizer
	flowFinalize   authored.Finalizer
	moduleFinalize imports.Finalizer
}

type shapeSpec struct {
	counts    [keyspace.FamilyCount]uint32
	rows      bodyOrder
	flow      authored.Input
	binds     []source.BindCells
	forms     []source.FunctionFormals
	nilOwners []keyspace.Term
	faults    []source.ControlFault
}

func openShapeFixture(t *testing.T, spec shapeSpec) *shapeFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 {
		t.Fatal("shape fixture requires an Entry Body")
	}
	flowInput := spec.flow
	flowInput.Counts = spec.counts

	sourceInput := shapeSourceInput(spec.counts, spec.rows, spec.binds, spec.forms, spec.nilOwners, spec.faults)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{Counts: spec.counts}
	if spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
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
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	fixture := &shapeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult, forest: forest,
		sourceFinalize: sourceFinalize, staticFinalize: staticFinalize,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(fixture.sourceFinalize, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}

func (f *shapeFixture) seal(t *testing.T) *Shape {
	t.Helper()
	shape, err := Seal(f.preimage, f.flow, f.bodies, f.binding, f.forest,
		f.staticFinalize.View().ContentID(), f.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("control.Shape Seal: %v", err)
	}
	return shape
}

func (f *shapeFixture) sealError() error {
	_, err := Seal(f.preimage, f.flow, f.bodies, f.binding, f.forest,
		f.staticFinalize.View().ContentID(), f.moduleFinalize.View().ContentID())
	return err
}

func shapeSourceInput(counts [keyspace.FamilyCount]uint32, rows bodyOrder, binds []source.BindCells, forms []source.FunctionFormals, nilOwners []keyspace.Term, faults []source.ControlFault) source.Input {
	input := source.Input{Name: "control-shape.lua"}
	input.Families = flowtest.FamilySpans(input.Name, counts)
	input.Bodies = make([]source.BodySource, counts[keyspace.FamilyBody])
	for index := range input.Bodies {
		var terms []keyspace.Term
		if index < len(rows) {
			terms = append(terms, rows[index]...)
		}
		input.Bodies[index] = source.BodySource{
			Body:  keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)),
			Terms: terms,
		}
	}
	input.Binds = make([]source.BindCells, counts[keyspace.FamilyBind])
	for index := range input.Binds {
		input.Binds[index].Bind = keyspace.MakeTerm(keyspace.FamilyBind, uint32(index+1))
		if index < len(binds) {
			input.Binds[index].Cells = append([]keyspace.Term(nil), binds[index].Cells...)
		}
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for index := range input.Functions {
		input.Functions[index].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		if index < len(forms) {
			input.Functions[index].Formals = append([]keyspace.Term(nil), forms[index].Formals...)
		}
	}
	input.Faults = append([]source.ControlFault(nil), faults...)
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input.Nil = flowtest.LiteralRows(counts[keyspace.FamilyNil], nilOwners, entry, func(owner keyspace.Term, _ uint32) source.NilLiteral {
		return source.NilLiteral{Owner: owner}
	})
	input.Bool = flowtest.LiteralRows(counts[keyspace.FamilyBool], nil, entry, func(owner keyspace.Term, ordinal uint32) source.BoolLiteral {
		return source.BoolLiteral{Owner: owner, Value: ordinal&1 == 1}
	})
	return input
}

// bodyOrder is test-only authored Source order. Production uses the typed
// source.BodySource relation; this alias keeps fixture syntax compact without
// creating a second Program API.
type bodyOrder [][]keyspace.Term

func bodyRows(rows ...[]keyspace.Term) bodyOrder { return rows }

func terms(counts [keyspace.FamilyCount]uint32, family keyspace.Family, ordinal uint32) keyspace.Term {
	if ordinal == 0 || ordinal > counts[family] {
		panic("shape fixture term outside count")
	}
	return keyspace.MakeTerm(family, ordinal)
}

func controlCounts(body, values, nils, cells, binds, loops, labels, gotos, breaks, functions uint32) (counts [keyspace.FamilyCount]uint32) {
	counts[keyspace.FamilyBody] = body
	counts[keyspace.FamilyValues] = values
	counts[keyspace.FamilyNil] = nils
	counts[keyspace.FamilyCell] = cells
	counts[keyspace.FamilyBind] = binds
	counts[keyspace.FamilyLoop] = loops
	counts[keyspace.FamilyLabel] = labels
	counts[keyspace.FamilyGoto] = gotos
	counts[keyspace.FamilyBreak] = breaks
	counts[keyspace.FamilyFunction] = functions
	return counts
}

func TestShapeRetainsExactControlRowsAndQueries(t *testing.T) {
	counts := controlCounts(2, 0, 1, 0, 0, 1, 2, 2, 1, 0)
	body1, body2 := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	loop1 := terms(counts, keyspace.FamilyLoop, 1)
	label1, label2 := terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyLabel, 2)
	goto1, goto2 := terms(counts, keyspace.FamilyGoto, 1), terms(counts, keyspace.FamilyGoto, 2)
	break1 := terms(counts, keyspace.FamilyBreak, 1)
	input := authored.Input{
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body1}, {Owner: body2}},
			Gotos:  []authored.Goto{{Owner: body1, Target: label1}, {Owner: body2, Target: label2}},
			Breaks: []authored.Break{{Owner: body2}},
			Loops:  []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopWhile, Control: terms(counts, keyspace.FamilyNil, 1)}},
		},
	}
	input.Counts = counts
	rows := bodyRows([]keyspace.Term{goto1, label1, loop1}, []keyspace.Term{goto2, label2, break1})
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input})
	shape := f.seal(t)
	if got, ok := shape.LabelBody(label1); !ok || got != body1 {
		t.Fatalf("LabelBody(label1) = %v/%v, want %v", got, ok, body1)
	}
	if got, ok := shape.LabelBody(label2); !ok || got != body2 {
		t.Fatalf("LabelBody(label2) = %v/%v, want %v", got, ok, body2)
	}
	if got, ok := shape.GotoTargetBody(goto1); !ok || got != body1 {
		t.Fatalf("GotoTargetBody(goto1) = %v/%v, want %v", got, ok, body1)
	}
	if got, ok := shape.GotoTargetBody(goto2); !ok || got != body2 {
		t.Fatalf("GotoTargetBody(goto2) = %v/%v, want %v", got, ok, body2)
	}
	if got, ok := shape.BreakLoop(break1); !ok || got != loop1 {
		t.Fatalf("BreakLoop(break1) = %v/%v, want %v", got, ok, loop1)
	}

}

func TestShapeAcceptsSameBodyForwardBackwardAndTrailingLabels(t *testing.T) {
	counts := controlCounts(1, 0, 0, 0, 0, 0, 3, 3, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	labels := []keyspace.Term{terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyLabel, 2), terms(counts, keyspace.FamilyLabel, 3)}
	gotos := []keyspace.Term{terms(counts, keyspace.FamilyGoto, 1), terms(counts, keyspace.FamilyGoto, 2), terms(counts, keyspace.FamilyGoto, 3)}
	input := authored.Input{Control: authored.ControlInput{
		Labels: []authored.Label{{Owner: body}, {Owner: body}, {Owner: body}},
		Gotos:  []authored.Goto{{Owner: body, Target: labels[1]}, {Owner: body, Target: labels[0]}, {Owner: body, Target: labels[2]}},
	}}
	input.Counts = counts
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{gotos[0], labels[0], gotos[1], labels[1], gotos[2], labels[2]}), flow: input})
	shape := f.seal(t)
	for _, label := range labels {
		if got, ok := shape.LabelBody(label); !ok || got != body {
			t.Fatalf("LabelBody(%v) = %v/%v, want %v", label, got, ok, body)
		}
	}
	for _, jump := range gotos {
		if got, ok := shape.GotoTargetBody(jump); !ok || got != body {
			t.Fatalf("GotoTargetBody(%v) = %v/%v, want %v", jump, got, ok, body)
		}
	}
}

func TestShapeRejectsForwardGotoOverRealBind(t *testing.T) {
	counts := controlCounts(1, 1, 1, 1, 1, 0, 2, 2, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	bind, cell := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyCell, 1)
	values, label, trailing := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyLabel, 2)
	jump, after := terms(counts, keyspace.FamilyGoto, 1), terms(counts, keyspace.FamilyGoto, 2)
	input := authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1)}},
		Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values}}},
		Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}, {Owner: body}}, Gotos: []authored.Goto{{Owner: body, Target: label}, {Owner: body, Target: label}}},
		Counts:  counts,
	}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{jump, bind, label, after, trailing}), flow: input, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}})
	if err := f.sealError(); err == nil {
		t.Fatal("forward Goto over a real Bind was accepted")
	}
}

func TestShapeAcceptsOrdinaryTerminalLabelAfterLocalScope(t *testing.T) {
	counts := controlCounts(1, 1, 1, 1, 1, 0, 1, 1, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	bind, cell := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyCell, 1)
	values, label, jump := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)
	input := authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1)}},
		Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values}}},
		Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}}, Gotos: []authored.Goto{{Owner: body, Target: label}}},
		Counts:  counts,
	}
	// A terminal label is outside an ordinary block's local scope. The same
	// source order that rejects an executable successor therefore remains legal
	// when the label is the final source occurrence.
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{jump, bind, label}), flow: input, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != body {
		t.Fatalf("GotoTargetBody(terminal) = %v/%v, want %v", got, ok, body)
	}
}

func TestShapeRejectsRepeatTerminalLabelThatRetainsLocal(t *testing.T) {
	counts := controlCounts(2, 1, 1, 1, 1, 1, 1, 1, 0, 0)
	body, repeatBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	loop, bind, cell := terms(counts, keyspace.FamilyLoop, 1), terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyCell, 1)
	values, label, jump := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)
	input := authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: repeatBody}}},
		Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: repeatBody}}, Binds: []authored.Bind{{Owner: repeatBody, Values: values}}},
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: repeatBody}}, Gotos: []authored.Goto{{Owner: repeatBody, Target: label}},
			Loops: []authored.Loop{{Owner: body, Body: repeatBody, Kind: kind.LoopRepeat, Control: terms(counts, keyspace.FamilyNil, 1)}},
		},
		Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{loop}, []keyspace.Term{jump, bind, label}), flow: input, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}, nilOwners: []keyspace.Term{repeatBody}})
	if err := f.sealError(); err == nil {
		t.Fatal("Repeat terminal label incorrectly discarded its local scope")
	}
}

func TestShapeAcceptsOutwardAncestorGoto(t *testing.T) {
	counts := controlCounts(3, 0, 1, 0, 0, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyBranch] = 1
	body, child := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	falseBody := terms(counts, keyspace.FamilyBody, 3)
	label, jump := terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)
	// A Branch is the only authored construct needed to create an ordinary
	// nested Body without introducing a Function activation boundary.
	branch := terms(counts, keyspace.FamilyBranch, 1)
	input := authored.Input{Control: authored.ControlInput{
		Labels: []authored.Label{{Owner: body}}, Gotos: []authored.Goto{{Owner: child, Target: label}},
		Branches: []authored.Branch{{Owner: body, Condition: terms(counts, keyspace.FamilyNil, 1), WhenTrue: child, WhenFalse: falseBody}},
	}, Counts: counts}
	rows := bodyRows([]keyspace.Term{label, branch}, []keyspace.Term{jump}, nil)
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != body {
		t.Fatalf("outward Goto target Body = %v/%v, want %v", got, ok, body)
	}
}

func TestShapeRejectsCrossFunctionGoto(t *testing.T) {
	counts := controlCounts(2, 1, 0, 0, 0, 0, 1, 1, 0, 1)
	counts[keyspace.FamilyReturn] = 1
	body, functionBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	function, label, jump := terms(counts, keyspace.FamilyFunction, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)
	values, returned := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyReturn, 1)
	input := authored.Input{
		Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{function}},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}, Labels: []authored.Label{{Owner: body}}, Gotos: []authored.Goto{{Owner: functionBody, Target: label}}},
		Counts:    counts,
	}
	rows := bodyRows([]keyspace.Term{returned, label}, []keyspace.Term{jump})
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input, forms: []source.FunctionFormals{{Function: function}}})
	if err := f.sealError(); err == nil {
		t.Fatal("cross-function Goto was accepted")
	}
}

func TestShapeSelectsNearestBreakLoop(t *testing.T) {
	counts := controlCounts(3, 0, 2, 0, 0, 2, 0, 0, 1, 0)
	body, innerBody, deepest := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2), terms(counts, keyspace.FamilyBody, 3)
	outer, inner, breakTerm := terms(counts, keyspace.FamilyLoop, 1), terms(counts, keyspace.FamilyLoop, 2), terms(counts, keyspace.FamilyBreak, 1)
	input := authored.Input{
		Control: authored.ControlInput{Breaks: []authored.Break{{Owner: deepest}}, Loops: []authored.Loop{
			{Owner: body, Body: innerBody, Kind: kind.LoopWhile, Control: terms(counts, keyspace.FamilyNil, 1)},
			{Owner: innerBody, Body: deepest, Kind: kind.LoopWhile, Control: terms(counts, keyspace.FamilyNil, 2)},
		}}, Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{outer}, []keyspace.Term{inner}, []keyspace.Term{breakTerm}), flow: input, nilOwners: []keyspace.Term{body, innerBody}})
	shape := f.seal(t)
	if got, ok := shape.BreakLoop(breakTerm); !ok || got != inner {
		t.Fatalf("BreakLoop = %v/%v, want nearest %v", got, ok, inner)
	}
}

func TestShapeRejectsBreakAcrossFunctionBoundary(t *testing.T) {
	counts := controlCounts(3, 2, 2, 0, 0, 1, 0, 0, 1, 1)
	counts[keyspace.FamilyReturn] = 2
	body, loopBody, functionBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2), terms(counts, keyspace.FamilyBody, 3)
	loop, function, returned1, returned2, breakTerm := terms(counts, keyspace.FamilyLoop, 1), terms(counts, keyspace.FamilyFunction, 1), terms(counts, keyspace.FamilyReturn, 1), terms(counts, keyspace.FamilyReturn, 2), terms(counts, keyspace.FamilyBreak, 1)
	values2 := terms(counts, keyspace.FamilyValues, 2)
	input := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: loopBody, Fixed: authored.Range{End: 1}}, {Owner: loopBody, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{function, terms(counts, keyspace.FamilyNil, 2)},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: loopBody, Body: functionBody}}},
		Control: authored.ControlInput{
			Breaks:  []authored.Break{{Owner: functionBody}},
			Returns: []authored.Return{{Owner: loopBody, Values: terms(counts, keyspace.FamilyValues, 1)}, {Owner: loopBody, Values: values2}},
			Loops:   []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: terms(counts, keyspace.FamilyNil, 1)}},
		},
		Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{loop}, []keyspace.Term{returned1, returned2}, []keyspace.Term{breakTerm}),
		flow:      input,
		forms:     []source.FunctionFormals{{Function: function}},
		nilOwners: []keyspace.Term{body, loopBody},
	})
	if err := f.sealError(); err == nil {
		t.Fatal("Break inside nested Function incorrectly selected the outer Loop")
	}
}

func TestShapeQueriesFailClosedAndDoNotAllocate(t *testing.T) {
	var nilShape *Shape
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	jump := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	breakTerm := keyspace.MakeTerm(keyspace.FamilyBreak, 1)
	for name, query := range map[string]func() (keyspace.Term, bool){
		"nil label": func() (keyspace.Term, bool) { return nilShape.LabelBody(label) },
		"nil break": func() (keyspace.Term, bool) { return nilShape.BreakLoop(breakTerm) },
		"nil goto":  func() (keyspace.Term, bool) { return nilShape.GotoTargetBody(jump) },
	} {
		if got, ok := query(); ok || got != 0 {
			t.Fatalf("%s = %v/%v, want zero/false", name, got, ok)
		}
	}

	counts := controlCounts(1, 0, 0, 0, 0, 0, 1, 1, 0, 0)
	input := authored.Input{Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}}, Gotos: []authored.Goto{{Owner: body, Target: label}}}, Counts: counts}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: bodyRows([]keyspace.Term{jump, label}), flow: input})
	shape := f.seal(t)
	wrong := []keyspace.Term{0, body, jump, breakTerm, keyspace.MakeTerm(keyspace.FamilyValues, 1)}
	for _, term := range wrong {
		if got, ok := shape.LabelBody(term); ok || got != 0 {
			t.Fatalf("LabelBody(%v) = %v/%v", term, got, ok)
		}
		if got, ok := shape.BreakLoop(term); ok || got != 0 {
			t.Fatalf("BreakLoop(%v) = %v/%v", term, got, ok)
		}
		if term != jump {
			if got, ok := shape.GotoTargetBody(term); ok || got != 0 {
				t.Fatalf("GotoTargetBody(%v) = %v/%v", term, got, ok)
			}
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_, _ = shape.LabelBody(label)
		_, _ = shape.GotoTargetBody(jump)
	}); allocs != 0 {
		t.Fatalf("control queries allocated %f times", allocs)
	}
}

func TestShapeHandlesDeepNestedBodiesIteratively(t *testing.T) {
	const depth = 512
	counts := controlCounts(depth, 0, depth-1, 0, 0, depth-1, 1, 1, 0, 0)
	root := terms(counts, keyspace.FamilyBody, 1)
	label, jump := terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)
	rows := make(bodyOrder, depth)
	loops := make([]authored.Loop, depth-1)
	for index := 0; index < depth-1; index++ {
		owner := terms(counts, keyspace.FamilyBody, uint32(index+1))
		child := terms(counts, keyspace.FamilyBody, uint32(index+2))
		loop := terms(counts, keyspace.FamilyLoop, uint32(index+1))
		rows[index] = []keyspace.Term{loop}
		loops[index] = authored.Loop{Owner: owner, Body: child, Kind: kind.LoopWhile, Control: terms(counts, keyspace.FamilyNil, uint32(index+1))}
	}
	rows[0] = append([]keyspace.Term{label}, rows[0]...)
	rows[depth-1] = []keyspace.Term{jump}
	input := authored.Input{
		Control: authored.ControlInput{Labels: []authored.Label{{Owner: root}}, Gotos: []authored.Goto{{Owner: terms(counts, keyspace.FamilyBody, depth), Target: label}}, Loops: loops},
		Counts:  counts,
	}
	nilOwners := make([]keyspace.Term, depth-1)
	for index := range nilOwners {
		nilOwners[index] = terms(counts, keyspace.FamilyBody, uint32(index+1))
	}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input, nilOwners: nilOwners})
	shape := f.seal(t)
	if got, ok := shape.GotoTargetBody(jump); !ok || got != root {
		t.Fatalf("deep outward Goto target = %v/%v, want %v", got, ok, root)
	}
}

// TestSealRejectsForeignBodyAtFunctionBoundary keeps the authored Function
// relation fixed while splicing a separately sealed Body result whose
// Function owner is a different lexical Body. Equal denominators are not
// enough: Control must cross-check the current Function→Body edge against the
// Body-owned parent and activation projections.
func TestSealRejectsForeignBodyAtFunctionBoundary(t *testing.T) {
	counts := controlCounts(3, 2, 2, 0, 0, 1, 0, 0, 0, 1)
	counts[keyspace.FamilyReturn] = 2
	body, loopBody, functionBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2), terms(counts, keyspace.FamilyBody, 3)
	loop, function, returned1, returned2 := terms(counts, keyspace.FamilyLoop, 1), terms(counts, keyspace.FamilyFunction, 1), terms(counts, keyspace.FamilyReturn, 1), terms(counts, keyspace.FamilyReturn, 2)
	values1, values2 := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyValues, 2)
	nil1, nil2 := terms(counts, keyspace.FamilyNil, 1), terms(counts, keyspace.FamilyNil, 2)

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: loopBody, Fixed: authored.Range{End: 1}}, {Owner: loopBody, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{function, nil2},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: loopBody, Body: functionBody}}},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: loopBody, Values: values1}, {Owner: loopBody, Values: values2}},
			Loops:   []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: nil1}},
		},
		Counts: counts,
	}
	current := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{loop}, []keyspace.Term{returned1, returned2}, nil),
		flow:      currentInput,
		forms:     []source.FunctionFormals{{Function: function}},
		nilOwners: []keyspace.Term{body, loopBody},
	})
	current.seal(t)

	foreignInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}},
			Terms: []keyspace.Term{function, nil2},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: body, Values: values1}, {Owner: body, Values: values2}},
			Loops:   []authored.Loop{{Owner: body, Body: loopBody, Kind: kind.LoopWhile, Control: nil1}},
		},
		Counts: counts,
	}
	foreign := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{loop, returned1, returned2}, nil, nil),
		flow:      foreignInput,
		forms:     []source.FunctionFormals{{Function: function}},
		nilOwners: []keyspace.Term{body, body},
	})
	if _, err := Seal(current.preimage, current.flow, foreign.bodies, current.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Body Function boundary was accepted")
	}
}

// TestSealRejectsBodyResultThatOmitsOrReordersBindRoot verifies that a Body
// proof cannot be substituted for the Body proof belonging to Source order.
// The current Source places a Goto before a Bind and its target Label after
// that Bind, so accepting a Body image which omits or moves the Bind would
// erase the local-scope barrier used by Goto validation.
func TestSealRejectsBodyResultThatOmitsOrReordersBindRoot(t *testing.T) {
	counts := controlCounts(2, 1, 1, 1, 1, 0, 1, 1, 0, 0)
	body, child := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	bind, cell := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyCell, 1)
	values, label, jump := terms(counts, keyspace.FamilyValues, 1), terms(counts, keyspace.FamilyLabel, 1), terms(counts, keyspace.FamilyGoto, 1)

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1)},
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
	current := openShapeFixture(t, shapeSpec{
		counts: counts,
		// Bind is between the Goto and its target Label. The child Body is a
		// second statement root so the foreign fixtures can omit/reorder Bind
		// while retaining every family denominator.
		rows:  bodyRows([]keyspace.Term{jump, bind, label, child}, nil),
		flow:  currentInput,
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
	})
	if err := current.sealError(); err == nil || !strings.Contains(err.Error(), "Goto enters") {
		t.Fatalf("baseline Goto-over-Bind fixture error = %v, want entered-local rejection", err)
	}

	cases := []struct {
		name      string
		rows      bodyOrder
		flow      authored.Input
		binds     []source.BindCells
		nilOwners []keyspace.Term
	}{
		{
			name: "bind omitted from current Body roots",
			// This honest Body proof places Bind in the child Body. Relative to
			// currentInput, the current Body's Bind root is therefore absent.
			rows: bodyRows(
				[]keyspace.Term{jump, label, child},
				[]keyspace.Term{bind},
			),
			flow: func() authored.Input {
				input := currentInput
				input.Values.Rows = []authored.Value{{Owner: child, Fixed: authored.Range{End: 1}}}
				input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: child}}
				input.Storage.Binds = []authored.Bind{{Owner: child, Values: values}}
				return input
			}(),
			binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
			nilOwners: []keyspace.Term{child},
		},
		{
			name: "bind reordered after child Body root",
			// Both fixtures own Bind by the entry Body, but the foreign Body
			// image orders the child Body root before Bind.
			rows: bodyRows(
				[]keyspace.Term{jump, child, bind, label},
				nil,
			),
			flow:  currentInput,
			binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			foreign := openShapeFixture(t, shapeSpec{
				counts:    counts,
				rows:      tc.rows,
				flow:      tc.flow,
				binds:     tc.binds,
				nilOwners: tc.nilOwners,
			})
			if _, err := Seal(current.preimage, current.flow, foreign.bodies, current.binding, current.forest,
				current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
				t.Fatal("foreign Body proof was accepted despite Source root mismatch")
			} else if !strings.Contains(err.Error(), "Body root") && !strings.Contains(err.Error(), "source position") && !strings.Contains(err.Error(), "Body provenance") {
				t.Fatalf("foreign Body proof rejection = %v, want semantic root/source mismatch", err)
			}
		})
	}
}

// TestSealRejectsForeignBindingAgainstBindCell uses two honest proofs with
// equal denominators. The fixtures swap the global and Bind-local Cell
// ordinals, so the current authored Source asks for its local Cell while the
// foreign Binding Result classifies that same ordinal as global.
func TestSealRejectsForeignBindingAgainstBindCell(t *testing.T) {
	counts := controlCounts(1, 1, 1, 2, 1, 0, 0, 0, 0, 0)
	body := terms(counts, keyspace.FamilyBody, 1)
	bind, values := terms(counts, keyspace.FamilyBind, 1), terms(counts, keyspace.FamilyValues, 1)
	nilTerm := terms(counts, keyspace.FamilyNil, 1)
	cell1, cell2 := terms(counts, keyspace.FamilyCell, 1), terms(counts, keyspace.FamilyCell, 2)
	exacts := []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}}

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{nilTerm},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: 1},
				{Kind: authored.CellLocal, Body: body},
			},
			Binds: []authored.Bind{{Owner: body, Values: values}},
		},
		Counts: counts,
	}
	current := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{bind}),
		flow:   currentInput,
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell2}}},
	}, exacts)

	foreignInput := currentInput
	foreignInput.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: body},
		{Kind: authored.CellGlobal, Key: 1},
	}
	foreign := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{bind}),
		flow:   foreignInput,
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell1}}},
	}, exacts)

	if _, err := Seal(current.preimage, current.flow, current.bodies, foreign.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Binding Result with global Bind Cell was accepted")
	} else if !strings.Contains(err.Error(), "Bind Cell scope") && !strings.Contains(err.Error(), "Binding provenance") {
		t.Fatalf("foreign Bind Cell rejection = %v, want semantic Bind Cell scope rejection", err)
	}
}

// TestSealRejectsForeignBindingAgainstLoopCell uses the same honest-result
// swap for a Loop host. The current authored Loop points at Cell 2; the
// foreign Binding Result classifies Cell 2 as global rather than CellLoop.
func TestSealRejectsForeignBindingAgainstLoopCell(t *testing.T) {
	counts := controlCounts(2, 1, 2, 2, 0, 1, 0, 0, 0, 0)
	body, loopBody := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	loop := terms(counts, keyspace.FamilyLoop, 1)
	cell1, cell2 := terms(counts, keyspace.FamilyCell, 1), terms(counts, keyspace.FamilyCell, 2)
	exacts := []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}}

	currentInput := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
			Terms: []keyspace.Term{terms(counts, keyspace.FamilyNil, 1), terms(counts, keyspace.FamilyNil, 2)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellGlobal, Key: 1},
			{Kind: authored.CellLocal, Body: loopBody},
		}},
		Control: authored.ControlInput{
			Loops: []authored.Loop{{
				Owner: body, Body: loopBody, Kind: kind.LoopNumericFor,
				Control: terms(counts, keyspace.FamilyValues, 1), Cells: authored.Range{End: 1},
			}},
			Cells: []keyspace.Term{cell2},
		},
		Counts: counts,
	}
	current := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{loop}, nil),
		flow:   currentInput,
	}, exacts)

	foreignInput := currentInput
	foreignInput.Storage.Cells = []authored.Cell{
		{Kind: authored.CellLocal, Body: loopBody},
		{Kind: authored.CellGlobal, Key: 1},
	}
	foreignInput.Control.Cells = []keyspace.Term{cell1}
	foreign := openShapeFixtureWithExactAtoms(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{loop}, nil),
		flow:   foreignInput,
	}, exacts)

	if _, err := Seal(current.preimage, current.flow, current.bodies, foreign.binding, current.forest,
		current.staticFinalize.View().ContentID(), current.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("foreign Binding Result with global Loop Cell was accepted")
	} else if !strings.Contains(err.Error(), "Loop Cell binding") && !strings.Contains(err.Error(), "Binding provenance") {
		t.Fatalf("foreign Loop Cell rejection = %v, want semantic Loop Cell binding rejection", err)
	}
}

// openShapeFixtureWithExactAtoms is the existing Shape fixture with the
// Source-owned exact atom batch exposed for cross-proof global-Cell laws. It
// intentionally still seals every owner proof; only the final Control call
// receives a foreign immutable result.
func openShapeFixtureWithExactAtoms(t *testing.T, spec shapeSpec, exactAtoms []keyspace.LiteralValue) *shapeFixture {
	t.Helper()
	if spec.counts[keyspace.FamilyBody] == 0 {
		t.Fatal("shape fixture requires an Entry Body")
	}
	flowInput := spec.flow
	flowInput.Counts = spec.counts

	sourceInput := shapeSourceInput(spec.counts, spec.rows, spec.binds, spec.forms, spec.nilOwners, spec.faults)
	sourceInput.ExactAtoms = append([]keyspace.LiteralValue(nil), exactAtoms...)
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticInput := static.Input{Counts: spec.counts}
	if spec.counts[keyspace.FamilyFunction] != 0 {
		staticInput.Contracts.Function = make([]staticcontracts.FunctionContract, spec.counts[keyspace.FamilyFunction])
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, static.Finalizer{}, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()

	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, authored.Finalizer{}, imports.Finalizer{})
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()

	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bodies, err := body.Seal(preimage, flowView, staticView, entry)
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
	moduleView := moduleFinalize.View()

	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		flowtest.CloseFinalizers(sourceFinalize, staticFinalize, flowFinalize, moduleFinalize)
		t.Fatalf("containment.Prove: %v", err)
	}
	fixture := &shapeFixture{
		preimage: preimage, flow: flowView, bodies: bodies, binding: bindingResult, forest: forest,
		sourceFinalize: sourceFinalize, staticFinalize: staticFinalize,
		flowFinalize: flowFinalize, moduleFinalize: moduleFinalize,
	}
	t.Cleanup(func() {
		flowtest.CloseFinalizers(fixture.sourceFinalize, fixture.staticFinalize, fixture.flowFinalize, fixture.moduleFinalize)
	})
	return fixture
}
func TestShapeAcceptsSourceControlFaultAlongsideAuthoredControl(t *testing.T) {
	counts := controlCounts(1, 0, 0, 0, 0, 0, 1, 1, 0, 0)
	counts[keyspace.FamilyControlFault] = 1
	body := terms(counts, keyspace.FamilyBody, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	gotoTerm := terms(counts, keyspace.FamilyGoto, 1)
	faultTerm := terms(counts, keyspace.FamilyControlFault, 1)
	input := authored.Input{Control: authored.ControlInput{
		Labels: []authored.Label{{Owner: body}},
		Gotos:  []authored.Goto{{Owner: body, Target: label}},
	}}
	input.Counts = counts
	fault := source.ControlFault{Owner: body, Kind: source.ControlFaultUndefinedGoto}
	f := openShapeFixture(t, shapeSpec{
		counts: counts,
		rows:   bodyRows([]keyspace.Term{label, faultTerm, gotoTerm}),
		flow:   input,
		faults: []source.ControlFault{fault},
	})
	shape := f.seal(t)

	order := f.preimage.Order()
	for index, want := range []keyspace.Term{label, faultTerm, gotoTerm} {
		if got, ok := order.BodyAt(body, index); !ok || got != want {
			t.Fatalf("Source Body order[%d] = %v/%v, want %v/true", index, got, ok, want)
		}
	}
	if got, ok := f.preimage.Faults().At(faultTerm); !ok || got != fault {
		t.Fatalf("Source Faults.At(%v) = %#v/%v, want %#v/true", faultTerm, got, ok, fault)
	}

	if got, ok := shape.LabelBody(label); !ok || got != body {
		t.Fatalf("LabelBody(label) = %v/%v, want %v/true", got, ok, body)
	}
	if got, ok := shape.GotoTargetBody(gotoTerm); !ok || got != body {
		t.Fatalf("GotoTargetBody(goto) = %v/%v, want %v/true", got, ok, body)
	}
	for name, query := range map[string]func() (keyspace.Term, bool){
		"LabelBody(fault)":      func() (keyspace.Term, bool) { return shape.LabelBody(faultTerm) },
		"BreakLoop(fault)":      func() (keyspace.Term, bool) { return shape.BreakLoop(faultTerm) },
		"GotoTargetBody(fault)": func() (keyspace.Term, bool) { return shape.GotoTargetBody(faultTerm) },
	} {
		if got, ok := query(); ok || got != 0 {
			t.Fatalf("%s = %v/%v, want zero/false", name, got, ok)
		}
	}
}
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
