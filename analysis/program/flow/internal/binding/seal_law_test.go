package binding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
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
	_, err, finish := trySealLawFixture(t, base, [][]keyspace.Term{{bind}}, []source.BindCells{{Bind: bind}}, nil, nil)
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
	_, err, finish = trySealLawFixture(t, global, [][]keyspace.Term{{}}, nil, nil,
		[]keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}})
	finish()
	if err == nil {
		t.Fatal("non-String global key was accepted")
	}
}

func TestResultQueriesAllocateNothing(t *testing.T) {
	result := Result{
		sourceID: bindingTestSourceID(), flowID: bindingTestFlowID(),
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
	result, err, finish := trySealLawFixture(t, input, rows, bindOrder, formalOrder, exactAtoms)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	return result, finish
}

func trySealLawFixture(t *testing.T, input authored.Input, rows [][]keyspace.Term, bindOrder []source.BindCells, formalOrder []source.FunctionFormals, exactAtoms []keyspace.LiteralValue) (Result, error, func()) {
	return trySealLawFixtureAtEntry(t, input, rows, bindOrder, formalOrder, exactAtoms, keyspace.MakeTerm(keyspace.FamilyBody, 1))
}

func trySealLawFixtureAtEntry(t *testing.T, input authored.Input, rows [][]keyspace.Term, bindOrder []source.BindCells, formalOrder []source.FunctionFormals, exactAtoms []keyspace.LiteralValue, entry keyspace.Term) (Result, error, func()) {
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
	return result, sealErr, finish
}
