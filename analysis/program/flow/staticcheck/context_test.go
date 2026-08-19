package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
)

func TestStaticCheckFunctionGenericHeaderAndFormalPhases(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	return1 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return2 := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	formalCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	readFormal := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readCapture := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	readHeader := keyspace.MakeTerm(keyspace.FamilyRead, 3)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	captureCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyCell, 3),
		checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyBind, 1),
		checkCount(keyspace.FamilyReturn, 2), checkCount(keyspace.FamilyRead, 3),
		checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyTypeParam, 1),
		checkCount(keyspace.FamilyTypeFunction, 1), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-function-phases.lua", counts: counts,
		rows:    [][]keyspace.Term{{bind, return1}, {return2}},
		binds:   []source.BindCells{{Bind: bind, Cells: []keyspace.Term{outerCell}}},
		formals: []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{formalCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{Start: 0, End: 1}}, {Owner: body1, Fixed: authored.Range{Start: 1, End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 3}}},
				Terms: []keyspace.Term{function, readFormal, readCapture},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body2}},
				Reads: []authored.Read{{Owner: body2, Source: formalCell}, {Owner: body2, Source: captureCell}, {Owner: body2, Source: formalCell}},
				Binds: []authored.Bind{{Owner: body1, Values: values1}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: captureCell, Outer: outerCell}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body1, Values: values2}, {Owner: body2, Values: values3}}},
		},
		static: static.Input{
			Declarations: staticdecl.Input{TypeParam: []staticdecl.TypeParam{{Owner: function, Name: 1}}},
			Signatures:   staticsig.Input{TypeFunction: []staticsig.TypeFunction{{Scope: function}}},
			Contracts:    staticcontracts.Input{Function: []staticcontracts.FunctionContract{{TypeParams: []keyspace.Term{param}}}},
			Operators:    staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: function, Operand: readHeader}}},
		},
	})
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	creation, ok := tree.pointForTerm(fixture.sourceView, function)
	if !ok {
		t.Fatal("Function has no creation Position")
	}
	header, ok := tree.pointAt(body2, 0)
	if !ok {
		t.Fatal("Function body has no header gap")
	}
	if tree.generic[keyspace.TermOrdinal(param)] != creation || !tree.paramVisible(creation, param) || !tree.paramVisible(header, param) {
		t.Fatalf("generic visibility = point %d creation %d header %d", tree.generic[keyspace.TermOrdinal(param)], creation, header)
	}
	if tree.cellVisible(creation, formalCell) || !tree.cellVisible(header, formalCell) {
		t.Fatalf("formal Cell visibility = creation %v/header %v", tree.cellVisible(creation, formalCell), tree.cellVisible(header, formalCell))
	}
	if tree.cellVisible(creation, outerCell) {
		t.Fatal("ordinary Bind Cell became visible before its Bind gap")
	}
	points := newObservationPoints(fixture.sourceView, fixture.flowView, fixture.forest, tree)
	if err := validateFunctionObservationAt(fixture.sourceView, fixture.flowView, points, function, true, body1); err != nil {
		t.Fatalf("generic observation: %v", err)
	}
	if err := validateFunctionObservationAt(fixture.sourceView, fixture.flowView, points, function, false, body2); err != nil {
		t.Fatalf("header observation: %v", err)
	}
	if err := validateFunctionCapturesAt(fixture.flowView, fixture.bindings, points, function); err != nil {
		t.Fatalf("self capture exception: %v", err)
	}
	if self, ok := fixture.bindings.FunctionCell(function); !ok || self != outerCell {
		t.Fatalf("FunctionCell = %v/%v, want %v/true", self, ok, outerCell)
	}
	if err := validateStaticReadAt(fixture.flowView, fixture.forest, fixture.bindings, points, readFormal); err != nil {
		t.Fatalf("formal Read visibility: %v", err)
	}
	if err := validateStaticReadAt(fixture.flowView, fixture.forest, fixture.bindings, points, readCapture); err != nil {
		t.Fatalf("capture Read visibility: %v", err)
	}
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("integrated Validate: %v", err)
	}
	if len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("integrated result = %#v", result)
	}
}

func TestStaticCheckValidateRejectsInvisibleNonSelfFunctionCapture(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	selfCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	badOuter := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	innerCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyCell, 3), checkCount(keyspace.FamilyBind, 2),
		checkCount(keyspace.FamilyReturn, 1), checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-invisible-nonself-capture.lua", counts: counts,
		rows: [][]keyspace.Term{{bind1, bind2}, {returnTerm}},
		binds: []source.BindCells{
			{Bind: bind1, Cells: []keyspace.Term{selfCell}},
			{Bind: bind2, Cells: []keyspace.Term{badOuter}},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 0}},
					{Owner: body1, Fixed: authored.Range{Start: 0, End: 0}},
					{Owner: body2, Fixed: authored.Range{Start: 0, End: 0}},
				},
				Terms: nil,
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
				},
				Binds: []authored.Bind{{Owner: body1, Values: values1}, {Owner: body1, Values: values2}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: innerCell, Outer: badOuter}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body2, Values: values3}}},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: selfCell, Operand: function}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	if !fixture.forest.Static(function) {
		t.Fatal("non-self capture Function occurrence is not static")
	}
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err == nil || len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("invisible non-self capture Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckRepeatUsesPositionAnchorNotFrontier(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	control := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	preReturn := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	ret := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	innerCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	values4 := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 3), checkCount(keyspace.FamilyLoop, 1),
		checkCount(keyspace.FamilyNil, 1), checkCount(keyspace.FamilyReturn, 2),
		checkCount(keyspace.FamilyValues, 4), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyBind, 2), checkCount(keyspace.FamilyCell, 3),
		checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-repeat-position.lua", counts: counts,
		// Repeat evaluates its scalar condition after the child Body.  The
		// source literal feeding that control therefore belongs to body2,
		// even though the Loop row itself is declared by body1.
		literalOwner: body2,
		rows:         [][]keyspace.Term{{preReturn, bind1, loop}, {bind2}, {ret}},
		binds:        []source.BindCells{{Bind: bind1, Cells: []keyspace.Term{outerCell}}, {Bind: bind2, Cells: []keyspace.Term{scopeCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body1}, {Owner: body1}, {Owner: body2}, {Owner: body3}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body3}},
				Binds: []authored.Bind{{Owner: body1, Values: values2}, {Owner: body2, Values: values3}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body2, Body: body3, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: innerCell, Outer: outerCell}},
			},
			Control: authored.ControlInput{
				Returns: []authored.Return{{Owner: body1, Values: values1}, {Owner: body3, Values: values4}},
				Loops:   []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopRepeat, Control: control}},
			},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	positionBody, _, positionCursor, ok := fixture.sourceView.Index().Position(loop)
	if !ok {
		t.Fatal("Repeat has no Position")
	}
	frontierBody, frontierCursor, ok := fixture.sourceView.Index().Frontier(loop)
	if !ok || frontierBody != body2 || frontierCursor == positionCursor {
		t.Fatalf("Repeat Position/Frontier = %v/%d and %v/%d", positionBody, positionCursor, frontierBody, frontierCursor)
	}
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	childBase := tree.bodies[keyspace.TermOrdinal(body2)].base
	want, ok := tree.pointAt(body1, uint32(positionCursor))
	if !ok || childBase != want {
		t.Fatalf("Repeat child base = %d, want Position point %d", childBase, want)
	}
	frontier, ok := tree.pointAt(frontierBody, uint32(frontierCursor))
	if ok && childBase == frontier {
		t.Fatal("Repeat child inherited Frontier instead of Position")
	}
	if result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	); err != nil || len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("Repeat integrated Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckNumericLoopCellsStartAtChildGapZero(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyRead, 1), checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyReturn, 1), checkCount(keyspace.FamilyLoop, 1),
		checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-numeric-loop-cell-phase.lua", counts: counts,
		rows: [][]keyspace.Term{{loop}, {returnTerm}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}, {Owner: body2, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{nil1, nil2},
			},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body2}}, Reads: []authored.Read{{Owner: body2, Source: cell}}},
			Control: authored.ControlInput{
				Loops: []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopNumericFor, Control: values1, Cells: authored.Range{End: 1}}}, Cells: []keyspace.Term{cell},
				Returns: []authored.Return{{Owner: body2, Values: values2}},
			},
		},
		static: static.Input{Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: cell, Operand: read}}}},
	})
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	childGap, ok := tree.pointAt(body2, 0)
	if !ok {
		t.Fatal("numeric loop body has no gap zero")
	}
	if got := tree.cellPoint[keyspace.TermOrdinal(cell)]; got != childGap {
		t.Fatalf("numeric loop Cell point = %d, want child gap zero %d", got, childGap)
	}
	if !tree.cellVisible(childGap, cell) {
		t.Fatal("numeric loop Cell is not visible at child gap zero")
	}
	if result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	); err != nil || len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("Loop integrated Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckChunkVarargIsVisibleAtEntryGapZero(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	ret := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyVararg, 1), checkCount(keyspace.FamilyNil, 1), checkCount(keyspace.FamilyValues, 1),
		checkCount(keyspace.FamilyReturn, 1), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-chunk-vararg.lua", counts: counts,
		rows: [][]keyspace.Term{{ret}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilValue}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Varargs: []authored.Vararg{{Owner: body, Cell: cell}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		},
		static: static.Input{Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: cell, Operand: vararg}}}},
	})
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	point, ok := tree.pointAt(body, 0)
	if !ok || point != tree.bodies[1].gapStart {
		t.Fatalf("chunk Vararg point = %d/%v, want entry gap zero %d", point, ok, tree.bodies[1].gapStart)
	}
	if !tree.cellVisible(point, cell) {
		t.Fatal("chunk Vararg Cell is not visible at entry gap zero")
	}
	if result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	); err != nil || len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("Chunk vararg integrated Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckBranchPositionAnchorsAndValidate(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	condition := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	innerCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	return2 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return4 := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	values4 := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 4), checkCount(keyspace.FamilyBranch, 1),
		checkCount(keyspace.FamilyNil, 1), checkCount(keyspace.FamilyReturn, 2),
		checkCount(keyspace.FamilyValues, 4), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyBind, 2), checkCount(keyspace.FamilyCell, 3),
		checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-branch-position.lua", counts: counts,
		rows:  [][]keyspace.Term{{bind1, branch}, {bind2}, {return2}, {return4}},
		binds: []source.BindCells{{Bind: bind1, Cells: []keyspace.Term{outerCell}}, {Bind: bind2, Cells: []keyspace.Term{scopeCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{{Owner: body1}, {Owner: body2}, {Owner: body3}, {Owner: body4}},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body4}},
				Binds: []authored.Bind{{Owner: body1, Values: values1}, {Owner: body2, Values: values2}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body2, Body: body4, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: innerCell, Outer: outerCell}},
			},
			Control: authored.ControlInput{
				Branches: []authored.Branch{{Owner: body1, Condition: condition, WhenTrue: body2, WhenFalse: body3}},
				Returns:  []authored.Return{{Owner: body3, Values: values3}, {Owner: body4, Values: values4}},
			},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	positionBody, _, cursor, ok := fixture.sourceView.Index().Position(branch)
	if !ok || positionBody != body1 {
		t.Fatalf("Branch Position = %v/%d/%v", positionBody, cursor, ok)
	}
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	want, ok := tree.pointAt(body1, uint32(cursor))
	if !ok {
		t.Fatal("Branch anchor gap is unavailable")
	}
	if tree.bodies[keyspace.TermOrdinal(body2)].base != want || tree.bodies[keyspace.TermOrdinal(body3)].base != want {
		t.Fatalf("Branch child bases = %d/%d, want %d", tree.bodies[keyspace.TermOrdinal(body2)].base, tree.bodies[keyspace.TermOrdinal(body3)].base, want)
	}
	if result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	); err != nil || len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("Branch integrated Validate = %#v/%v", result, err)
	}
}
