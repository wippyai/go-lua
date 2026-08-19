package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestSealClassifiesEveryCellRoleAndHost(t *testing.T) {
	terms := make([]keyspace.Term, 7)
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
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 7, keyspace.FamilyBind: 1,
		keyspace.FamilyValues: 1, keyspace.FamilyFunction: 1, keyspace.FamilyLoop: 1,
		keyspace.FamilyVararg: 2, keyspace.FamilyNil: 2,
	}}
	input.Values.Rows = []authored.Value{{Owner: body1, Fixed: authored.Range{Start: 0, End: 2}}}
	input.Values.Terms = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellGlobal, Key: 1},
		{Kind: authored.CellLocal, Body: body1},
		{Kind: authored.CellLocal, Body: body1},
		{Kind: authored.CellLocal, Body: body3},
		{Kind: authored.CellLocal, Body: body2},
		{Kind: authored.CellLocal, Body: body2},
		{Kind: authored.CellLocal, Body: body2},
	}
	input.Storage.Varargs = []authored.Vararg{{Owner: body3, Cell: terms[2]}, {Owner: body2, Cell: terms[5]}}
	input.Storage.Binds = []authored.Bind{{Owner: body1, Values: values}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: terms[5], Captures: authored.Range{Start: 0, End: 1}}}
	input.Functions.Captures = []authored.Capture{{Inner: terms[6], Outer: terms[1]}}
	input.Control.Loops = []authored.Loop{{Owner: body1, Body: body3, Kind: kind.LoopNumericFor, Control: values, Cells: authored.Range{Start: 0, End: 1}}}
	input.Control.Cells = []keyspace.Term{terms[3]}

	rows := [][]keyspace.Term{{bind, loop}, nil, nil}
	result, finish := sealLawFixture(t, input, rows,
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{terms[1]}}},
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{terms[4]}}},
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
	)
	defer finish()

	wantRoles := []kind.CellRole{kind.CellGlobal, kind.CellLocal, kind.CellChunkVararg, kind.CellLoop, kind.CellFormal, kind.CellFunctionVararg, kind.CellCapture}
	wantHosts := []keyspace.Term{0, bind, body1, loop, function, function, function}
	if result.CellCount() != len(terms) {
		t.Fatalf("CellCount = %d, want %d", result.CellCount(), len(terms))
	}
	for index, cell := range terms {
		role, ok := result.Role(cell)
		if !ok || role != wantRoles[index] {
			t.Fatalf("Role(%v) = %v/%v, want %v", cell, role, ok, wantRoles[index])
		}
		host, ok := result.Host(cell)
		if !ok || host != wantHosts[index] {
			t.Fatalf("Host(%v) = %v/%v, want %v", cell, host, ok, wantHosts[index])
		}
	}
	if chunk, ok := result.ChunkVararg(); !ok || chunk != terms[2] {
		t.Fatalf("ChunkVararg = %v/%v, want %v/true", chunk, ok, terms[2])
	}
}

func TestSealRejectsEmptyBindAndNonStringGlobal(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	base := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyCell: 1, keyspace.FamilyBind: 1, keyspace.FamilyValues: 1,
	}}
	base.Values.Rows = []authored.Value{{Owner: body}}
	base.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body}}
	base.Storage.Binds = []authored.Bind{{Owner: body, Values: values}}
	_, finish, err := trySealLawFixture(t, base, [][]keyspace.Term{{bind}}, []source.BindCells{{Bind: bind}}, nil, nil)
	finish()
	if err == nil {
		t.Fatal("empty Bind order was accepted")
	}
	// The same authored relation is accepted once Source supplies its required
	// nonempty Cell order.
	good, finish := sealLawFixture(t, base, [][]keyspace.Term{{bind}}, []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}, nil, nil)
	finish()
	if good.CellCount() != 1 {
		t.Fatalf("nonempty Bind fixture did not classify Cell")
	}

	global := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	global.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	_, finish, err = trySealLawFixture(t, global, [][]keyspace.Term{{}}, nil, nil,
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}})
	finish()
	if err == nil {
		t.Fatal("non-String global key was accepted")
	}
}

func TestResultQueriesAllocateNothing(t *testing.T) {
	result := Result{
		sourceID: flowtest.ContentIDAt(0x11), flowID: flowtest.ContentIDAt(0x22),
		roles: []kind.CellRole{0, kind.CellChunkVararg}, hosts: []keyspace.Term{0, 1},
		chunk: keyspace.MakeTerm(keyspace.FamilyCell, 1),
	}
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = result.Role(cell)
		_, _ = result.Host(cell)
		_, _ = result.ChunkVararg()
		_ = result.CellCount()
	})
	if allocs != 0 {
		t.Fatalf("queries allocate %.2f times", allocs)
	}
}

func sealLawFixture(t *testing.T, input authored.Input, rows [][]keyspace.Term, bindOrder []source.BindCells, formalOrder []source.FunctionFormals, exactAtoms []keyspace.LiteralValue) (Result, func()) {
	t.Helper()
	result, finish, err := trySealLawFixture(t, input, rows, bindOrder, formalOrder, exactAtoms)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	return result, finish
}

func trySealLawFixture(t *testing.T, input authored.Input, rows [][]keyspace.Term, bindOrder []source.BindCells, formalOrder []source.FunctionFormals, exactAtoms []keyspace.LiteralValue) (Result, func(), error) {
	return trySealLawFixtureAtEntry(t, input, rows, bindOrder, formalOrder, exactAtoms, keyspace.MakeTerm(keyspace.FamilyBody, 1))
}

func trySealLawFixtureAtEntry(t *testing.T, input authored.Input, rows [][]keyspace.Term, bindOrder []source.BindCells, formalOrder []source.FunctionFormals, exactAtoms []keyspace.LiteralValue, entry keyspace.Term) (Result, func(), error) {
	t.Helper()
	input.Counts[keyspace.FamilyBody] = uint32(len(rows))
	flowDraft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinish, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinish.View()
	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinish, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinish.View()

	name := "binding-law.lua"
	sourceInput := source.Input{Name: name, Binds: bindOrder, Functions: formalOrder, ExactAtoms: exactAtoms}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := int(input.Counts[family])
		spans := make([]source.Span, count)
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		sourceInput.Families = append(sourceInput.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceInput.Bodies = make([]source.BodySource, len(rows))
	for index, bodyTerms := range rows {
		sourceInput.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)), Terms: bodyTerms}
	}
	for index := 0; index < int(input.Counts[keyspace.FamilyNil]); index++ {
		sourceInput.Nil = append(sourceInput.Nil, source.NilLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)})
	}
	for index := 0; index < int(input.Counts[keyspace.FamilyBool]); index++ {
		sourceInput.Bool = append(sourceInput.Bool, source.BoolLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)})
	}
	for index := 0; index < int(input.Counts[keyspace.FamilyInteger]); index++ {
		sourceInput.Integer = append(sourceInput.Integer, source.IntegerLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)})
	}
	for index := 0; index < int(input.Counts[keyspace.FamilyFloat]); index++ {
		sourceInput.Float = append(sourceInput.Float, source.FloatLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bits: uint64(index + 1)})
	}
	for index := 0; index < int(input.Counts[keyspace.FamilyString]); index++ {
		sourceInput.String = append(sourceInput.String, source.StringLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: "literal"})
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinish, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinish.Preimage()
	bodyResult, err := body.Seal(preimage, flowView, staticView, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, sealErr := Seal(preimage, flowView, bodyResult, entry)
	finish := func() {
		_ = flowFinish.Abort()
		_ = staticFinish.Abort()
		_ = sourceFinish.Abort()
	}
	return result, finish, sealErr
}
func TestSealRejectsZeroInvalidAndNonParentlessEntries(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyCell: 1, keyspace.FamilyBind: 1, keyspace.FamilyValues: 1,
	}}
	input.Values.Rows = []authored.Value{{Owner: body}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body}}
	input.Storage.Binds = []authored.Bind{{Owner: body, Values: values}}
	order := []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}

	for _, entry := range []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyBody, 2)} {
		_, finish, err := trySealLawFixtureAtEntry(t, input, [][]keyspace.Term{{bind}}, order, nil, nil, entry)
		finish()
		if err == nil {
			t.Fatalf("entry %v was accepted", entry)
		}
	}
}

func TestSealRejectsSelectingAValidNonParentlessBodyAsEntry(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 2, keyspace.FamilyFunction: 1}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2}}
	_, finish, err := trySealLawFixtureAtEntry(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function}}, nil, body2)
	finish()
	if err == nil {
		t.Fatal("valid non-parentless Body was accepted as Entry")
	}
}
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
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
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
	_, finish, err := trySealLawFixture(t, input, rows, nil,
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
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{bind}, {}, {}},
		[]source.BindCells{{Bind: bind, Cells: []keyspace.Term{outer}}},
		[]source.FunctionFormals{{Function: function1}, {Function: function2}}, nil)
	finish()
	if err == nil {
		t.Fatal("Capture Outer across an intervening Function activation was accepted")
	}
}
func TestSealAcceptsRepeatedChunkOccurrenceFromNestedNonFunctionBodies(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 1, keyspace.FamilyBranch: 1,
		keyspace.FamilyNil: 1, keyspace.FamilyVararg: 2,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell}, {Owner: body3, Cell: cell}}
	input.Control.Branches = []authored.Branch{{Owner: body1, Condition: nilTerm, WhenTrue: body2, WhenFalse: body3}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{branch}, {}, {}}, nil, nil, nil)
	defer finish()
	if role, ok := result.Role(cell); !ok || role != kind.CellChunkVararg {
		t.Fatalf("Role(chunk) = %v/%v", role, ok)
	}
	if chunk, ok := result.ChunkVararg(); !ok || chunk != cell {
		t.Fatalf("ChunkVararg = %v/%v, want %v/true", chunk, ok, cell)
	}
}

func TestSealRejectsConflictingChunkOccurrenceCells(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 3, keyspace.FamilyCell: 2, keyspace.FamilyBranch: 1,
		keyspace.FamilyNil: 1, keyspace.FamilyVararg: 2,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body1}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell1}, {Owner: body3, Cell: cell2}}
	input.Control.Branches = []authored.Branch{{Owner: body1, Condition: keyspace.MakeTerm(keyspace.FamilyNil, 1), WhenTrue: body2, WhenFalse: body3}}
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{branch}, {}, {}}, nil, nil, nil)
	finish()
	if err == nil {
		t.Fatal("conflicting chunk occurrence Cells were accepted")
	}
}

func TestSealRejectsNonzeroActivationVarargProviderMismatch(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyFunction: 1, keyspace.FamilyVararg: 1,
	}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body2}, {Kind: authored.CellLocal, Body: body2}}
	input.Storage.Varargs = []authored.Vararg{{Owner: body2, Cell: cell2}}
	input.Functions.Rows = []authored.Function{{Owner: body1, Body: body2, Vararg: cell1}}
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function}}, nil)
	finish()
	if err == nil {
		t.Fatal("Vararg occurrence mismatching Function.Vararg was accepted")
	}
}
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
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{bind}, nil},
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
		sourceID:      flowtest.ContentIDAt(0x11),
		flowID:        flowtest.ContentIDAt(0x22),
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
func TestSealRejectsInvalidAndEmptyGlobalExactStringAtoms(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	cases := []struct {
		name  string
		atoms []keyspace.LiteralValue
	}{
		{name: "missing"},
		{name: "integer", atoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}}},
		{name: "empty-string", atoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: ""}}},
	}
	for _, test := range cases {
		_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil, test.atoms)
		finish()
		if err == nil {
			t.Fatalf("%s global atom was accepted", test.name)
		}
	}
}

func TestAuthoredRejectsDuplicateGlobalDenseKeys(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 2}}
	input.Storage.Cells = []authored.Cell{
		{Kind: authored.CellGlobal, Key: 1},
		{Kind: authored.CellGlobal, Key: 1},
	}
	if _, err := authored.Build(input); err == nil {
		t.Fatal("duplicate global dense keys were admitted to authored storage")
	}
}

func TestSealGlobalRoleHasZeroHost(t *testing.T) {
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellGlobal, Key: 1}}
	result, finish := sealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil,
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "g"}})
	defer finish()
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if role, ok := result.Role(cell); !ok || role != kind.CellGlobal {
		t.Fatalf("Role(global) = %v/%v", role, ok)
	}
	if host, ok := result.Host(cell); !ok || host != 0 {
		t.Fatalf("Host(global) = %v/%v, want 0/true", host, ok)
	}
}
func TestSealRejectsExpiredSourcePreimageIndependently(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	defer func() { _ = flowFinalizer.Abort() }()
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("expired Source Preimage was accepted")
	}
}

func TestSealRejectsExpiredAuthoredViewIndependently(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	defer func() { _ = sourceFinalizer.Abort() }()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("expired authored View was accepted")
	}
}

func TestSealExpiredOwnersOnZeroFamilyEmptyModel(t *testing.T) {
	bodies, bodyFinish := liveLifecycleBodyResult(t)
	defer bodyFinish()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{})
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("zero-family model with expired Source was accepted")
	}
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}

	preimage, sourceFinalizer = liveLifecycleSource(t, 1)
	view, flowFinalizer = liveLifecycleAuthored(t, authored.Input{})
	if err := flowFinalizer.Abort(); err != nil {
		t.Fatalf("authored Abort: %v", err)
	}
	if _, err := Seal(preimage, view, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil {
		t.Fatal("zero-family model with expired authored View was accepted")
	}
	if err := sourceFinalizer.Abort(); err != nil {
		t.Fatalf("source Abort: %v", err)
	}
}

func liveLifecycleBodyResult(t *testing.T) (*body.Result, func()) {
	t.Helper()
	view, flowFinalizer := liveLifecycleAuthored(t, authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1}})
	preimage, sourceFinalizer := liveLifecycleSource(t, 1)
	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	result, err := body.Seal(preimage, view, staticFinalizer.View(), keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		_ = flowFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		_ = staticFinalizer.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	return result, func() {
		_ = flowFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		_ = staticFinalizer.Abort()
	}
}

func liveLifecycleAuthored(t *testing.T, input authored.Input) (authored.View, authored.Finalizer) {
	t.Helper()
	draft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	finish, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	return finish.View(), finish
}

func liveLifecycleSource(t *testing.T, bodyCount int) (source.Preimage, source.Finalizer) {
	t.Helper()
	name := "binding-lifecycle.lua"
	input := source.Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		if family == keyspace.FamilyBody {
			count = bodyCount
		}
		spans := make([]source.Span, count)
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = make([]source.BodySource, bodyCount)
	for index := range input.Bodies {
		input.Bodies[index].Body = keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	}
	draft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	finish, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	return finish.Preimage(), finish
}
func TestSealRejectsUnclassifiedAndDoubleClaimedCells(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input := authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyCell: 1}}
	input.Storage.Cells = []authored.Cell{{Kind: authored.CellLocal, Body: body}}
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{}}, nil, nil, nil)
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
	_, finish, err = trySealLawFixture(t, input, [][]keyspace.Term{{bind}},
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
	_, finish, err := trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
		[]source.FunctionFormals{{Function: function, Formals: []keyspace.Term{cell}}}, nil)
	finish()
	if err == nil {
		t.Fatal("formal Cell with wrong defining Body was accepted")
	}

	input.Storage.Cells[0].Body = body2
	input.Functions.Rows[0].Vararg = cell
	_, finish, err = trySealLawFixture(t, input, [][]keyspace.Term{{}, {}}, nil,
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
