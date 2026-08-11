package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

type portsFixture struct {
	identity       source.Identity
	sourceFinalize source.Finalizer
	view           authored.View
	flowFinalize   authored.Finalizer
	staticFinalize static.Finalizer
	moduleFinalize module.Finalizer
	forest         *containment.Result
}

func (fixture *portsFixture) close() {
	_ = fixture.moduleFinalize.Abort()
	_ = fixture.flowFinalize.Abort()
	_ = fixture.staticFinalize.Abort()
	_ = fixture.sourceFinalize.Abort()
}

func directSourceRoots(flow authored.Input) [][]keyspace.Term {
	roots := make([][]keyspace.Term, flow.Counts[keyspace.FamilyBody])
	appendRoot := func(owner, term keyspace.Term) {
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
			return
		}
		index := int(keyspace.TermOrdinal(owner) - 1)
		if index >= 0 && index < len(roots) {
			roots[index] = append(roots[index], term)
		}
	}
	for ordinal, row := range flow.Storage.Binds {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Storage.Assigns {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyAssign, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Calls {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyCall, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Returns {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyReturn, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Breaks {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyBreak, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Labels {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyLabel, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Gotos {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyGoto, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Branches {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyBranch, uint32(ordinal+1)))
	}
	for ordinal, row := range flow.Control.Loops {
		appendRoot(row.Owner, keyspace.MakeTerm(keyspace.FamilyLoop, uint32(ordinal+1)))
	}
	return roots
}

func openPortsFixture(t *testing.T, counts [keyspace.FamilyCount]uint32, flow authored.Input, nilCount int) portsFixture {
	t.Helper()
	if nilCount != int(counts[keyspace.FamilyNil]) {
		t.Fatal("nil fixture cardinality mismatch")
	}
	flow.Counts = counts
	families := make([]source.FamilySpans, int(keyspace.FamilyCount)-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			spans[index] = source.Span{File: "ports.lua", StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
		}
		families[int(family)-1] = source.FamilySpans{Family: family, Spans: spans}
	}
	input := source.Input{Name: "ports.lua", Families: families}
	input.Bodies = make([]source.BodySource, counts[keyspace.FamilyBody])
	for index := range input.Bodies {
		input.Bodies[index] = source.BodySource{Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))}
	}
	input.Nil = make([]source.NilLiteral, nilCount)
	for index := range input.Nil {
		input.Nil[index].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	}
	// Repeat controls are evaluated after their lexical Body. Keep synthetic
	// Source literal ownership aligned with that production frontier.
	for _, loop := range flow.Control.Loops {
		if loop.Kind != kind.LoopRepeat || keyspace.TermFamily(loop.Control) != keyspace.FamilyNil {
			continue
		}
		ordinal := keyspace.TermOrdinal(loop.Control)
		if ordinal != 0 && uint64(ordinal) <= uint64(len(input.Nil)) {
			input.Nil[ordinal-1].Owner = loop.Body
		}
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBool]; ordinal++ {
		input.Bool = append(input.Bool, source.BoolLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: ordinal%2 == 1})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyInteger]; ordinal++ {
		input.Integer = append(input.Integer, source.IntegerLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: int64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFloat]; ordinal++ {
		input.Float = append(input.Float, source.FloatLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bits: uint64(ordinal)})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyString]; ordinal++ {
		input.String = append(input.String, source.StringLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: "literal"})
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyKey]; ordinal++ {
		if ordinal%2 == 1 {
			text := "key" + string(rune('a'+ordinal-1))
			input.Keys = append(input.Keys, source.NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), text))
			input.ExactAtoms = append(input.ExactAtoms, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text})
		} else {
			value := int64(ordinal)
			input.Keys = append(input.Keys, source.ListKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), value))
			input.ExactAtoms = append(input.ExactAtoms, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value})
		}
	}
	for _, cell := range flow.Storage.Cells {
		if cell.Kind != authored.CellGlobal || cell.Key == 0 {
			continue
		}
		for len(input.ExactAtoms) < int(cell.Key) {
			input.ExactAtoms = append(input.ExactAtoms, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
		}
	}
	input.Functions = make([]source.FunctionFormals, counts[keyspace.FamilyFunction])
	for ordinal := range input.Functions {
		input.Functions[ordinal].Function = keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal+1))
	}
	for bodyOrdinal, roots := range directSourceRoots(flow) {
		if bodyOrdinal < len(input.Bodies) {
			input.Bodies[bodyOrdinal].Terms = roots
		}
	}
	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	finalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	identity := finalize.Preimage().Identity()
	staticInput := static.Input{Counts: counts}
	if primitiveCount := int(counts[keyspace.FamilyTypePrimitive]); primitiveCount != 0 {
		staticInput.Types.Primitive = make([]static.Primitive, primitiveCount)
		for index := range staticInput.Types.Primitive {
			staticInput.Types.Primitive[index] = static.Primitive{Kind: static.PrimitiveNumber}
		}
	}
	if typeValueCount := int(counts[keyspace.FamilyTypeValue]); typeValueCount != 0 {
		// Keep synthetic TypeValue controls concrete so the fixture exercises
		// authored Values/Loop ownership rather than an unclassified static
		// operand. Each target has its own primitive declaration so the static
		// ownership relation remains one-to-one.
		if counts[keyspace.FamilyTypePrimitive] == 0 {
			_ = finalize.Abort()
			t.Fatalf("TypeValue fixture requires a primitive target")
		}
		staticInput.Operands.TypeValue = make([]static.TypeValueTarget, typeValueCount)
		for index := range staticInput.Operands.TypeValue {
			staticInput.Operands.TypeValue[index] = static.TypeValueTarget{
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, uint32(index+1)),
			}
		}
	}
	if functionCount := int(counts[keyspace.FamilyFunction]); functionCount != 0 {
		staticInput.Contracts.Function = make([]static.FunctionContract, functionCount)
	}
	if callCount := int(counts[keyspace.FamilyCall]); callCount != 0 {
		staticInput.Contracts.Call = make([]static.CallContract, callCount)
	}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = finalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = finalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	staticView := staticFinalize.View()
	flowDraft, err := authored.Build(flow)
	if err != nil {
		_ = finalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = finalize.Abort()
		_ = staticFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView := flowFinalize.View()
	bodies, err := body.Seal(finalize.Preimage(), flowView, staticView, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		_ = finalize.Abort()
		_ = staticFinalize.Abort()
		_ = flowFinalize.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := binding.Seal(finalize.Preimage(), flowView, bodies, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		_ = finalize.Abort()
		_ = staticFinalize.Abort()
		_ = flowFinalize.Abort()
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("module.Finalizer: %v", err)
	}
	moduleView := moduleFinalize.View()
	forest, _, err := containment.Prove(finalize.Preimage(), staticView, flowView, bodies, bindings, moduleView, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		_ = moduleFinalize.Abort()
		_ = finalize.Abort()
		_ = staticFinalize.Abort()
		_ = flowFinalize.Abort()
		t.Fatalf("containment.Prove: %v", err)
	}
	view, err := flowFinalize.Commit()
	if err != nil {
		t.Fatalf("authored.Commit: %v", err)
	}
	return portsFixture{identity: identity, sourceFinalize: finalize, view: view, flowFinalize: flowFinalize, staticFinalize: staticFinalize, moduleFinalize: moduleFinalize, forest: forest}
}

func TestPortsLeafQueriesAreAllocationFree(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilTerm}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 1)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := ports.Entry(nilTerm); !ok || entry != nilTerm {
		t.Fatalf("Entry(nil) = %v, %v", entry, ok)
	}
	if finish, ok := ports.Finish(nilTerm); !ok || finish != nilTerm {
		t.Fatalf("Finish(nil) = %v, %v", finish, ok)
	}
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	if _, ok := ports.Entry(key); ok {
		t.Fatal("static Key unexpectedly received an Entry port")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = ports.Entry(nilTerm)
		_, _ = ports.Finish(nilTerm)
	}); allocations != 0 {
		t.Fatalf("port queries allocate %v times", allocations)
	}
}

func TestPortsResolveDeepEntryIteratively(t *testing.T) {
	const depth = 2048
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	rows := make([]authored.Unary, depth)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyUnary: depth, keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1}
	for index := range rows {
		operand := keyspace.MakeTerm(keyspace.FamilyNil, 1)
		if index != 0 {
			operand = keyspace.MakeTerm(keyspace.FamilyUnary, uint32(index))
		}
		rows[index] = authored.Unary{Owner: body, Op: kind.UnaryNeg, Operand: operand}
	}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyUnary, depth)}},
		Operators: authored.OperatorsInput{Unaries: rows},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 1)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	outer := keyspace.MakeTerm(keyspace.FamilyUnary, depth)
	if entry, ok := ports.Entry(outer); !ok || entry != keyspace.MakeTerm(keyspace.FamilyNil, 1) {
		t.Fatalf("deep Entry = %v, %v", entry, ok)
	}
	deepTerm := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
	if err := requireOwner(fixture.view, fixture.forest, body, deepTerm); err != nil {
		t.Fatalf("deep owner proof rejected the canonical Body: %v", err)
	}
	ownerProofOK := true
	if allocations := testing.AllocsPerRun(100, func() {
		if err := requireOwner(fixture.view, fixture.forest, body, deepTerm); err != nil {
			ownerProofOK = false
		}
	}); allocations != 0 {
		t.Fatalf("deep owner proof allocated %v objects per run", allocations)
	}
	if !ownerProofOK {
		t.Fatal("deep owner proof became unstable during allocation probe")
	}
}

func TestPortsWideValuesEntryCursorIsNotByteSized(t *testing.T) {
	const width = 300
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	terms := make([]keyspace.Term, width)
	for index := range terms {
		terms[index] = keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+1))
	}
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: width,
		keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1,
	}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: width}}}, Terms: terms},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, width)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := ports.Entry(values); !ok || entry != terms[0] {
		t.Fatalf("wide Values Entry = %v/%v, want %v", entry, ok, terms[0])
	}
}

// openLocalCellWriteFixture builds one lexical chain with a loop-local Cell
// (Body3), a descendant loop Body4, a sibling loop Body5, and the enclosing
// Body2. Numeric controls use concrete TypeValues so this law reaches
// writableTarget rather than failing on an unrelated scalar classification.
func openLocalCellWriteFixture(t *testing.T, assignmentOwner keyspace.Term) portsFixture {
	t.Helper()
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	body5 := keyspace.MakeTerm(keyspace.FamilyBody, 5)
	values := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyValues, ordinal) }
	typeValue := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyTypeValue, ordinal) }
	cell := func(ordinal uint32) keyspace.Term { return keyspace.MakeTerm(keyspace.FamilyCell, ordinal) }
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)

	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 5, keyspace.FamilyValues: 5,
		keyspace.FamilyTypeValue: 8, keyspace.FamilyTypePrimitive: 8,
		keyspace.FamilyCell: 4, keyspace.FamilyLoop: 4,
		keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 1,
	}
	terms := make([]keyspace.Term, 0, 8)
	typeValues := make([]authored.TypeValue, 0, 8)
	for ordinal := uint32(1); ordinal <= 8; ordinal++ {
		terms = append(terms, typeValue(ordinal))
		owner := body2
		switch {
		case ordinal <= 2:
			owner = body1
		case ordinal <= 4:
			owner = body2
		case ordinal <= 6:
			owner = body3
		}
		typeValues = append(typeValues, authored.TypeValue{Owner: owner})
	}
	flow := authored.Input{
		TypeValues: typeValues,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body1, Fixed: authored.Range{Start: 0, End: 2}},
				{Owner: body2, Fixed: authored.Range{Start: 2, End: 4}},
				{Owner: body3, Fixed: authored.Range{Start: 4, End: 6}},
				{Owner: body2, Fixed: authored.Range{Start: 6, End: 8}},
				{Owner: assignmentOwner, Fixed: authored.Range{Start: 8, End: 8}},
			},
			Terms: terms,
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: body2},
				{Kind: authored.CellLocal, Body: body3},
				{Kind: authored.CellLocal, Body: body4},
				{Kind: authored.CellLocal, Body: body5},
			},
			Assigns: []authored.Assign{{Owner: assignmentOwner, Values: values(5)}},
			Writes:  []authored.Write{{Assign: assign, Target: cell(2)}},
		},
		Control: authored.ControlInput{
			Loops: []authored.Loop{
				{Owner: body1, Body: body2, Kind: kind.LoopNumericFor, Control: values(1), Cells: authored.Range{Start: 0, End: 1}},
				{Owner: body2, Body: body3, Kind: kind.LoopNumericFor, Control: values(2), Cells: authored.Range{Start: 1, End: 2}},
				{Owner: body3, Body: body4, Kind: kind.LoopNumericFor, Control: values(3), Cells: authored.Range{Start: 2, End: 3}},
				{Owner: body2, Body: body5, Kind: kind.LoopNumericFor, Control: values(4), Cells: authored.Range{Start: 3, End: 4}},
			},
			Cells: []keyspace.Term{cell(1), cell(2), cell(3), cell(4)},
		},
	}
	return openPortsFixture(t, counts, flow, 0)
}

func TestPortsLocalCellWriteOwnershipMatrix(t *testing.T) {
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	body4 := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	body5 := keyspace.MakeTerm(keyspace.FamilyBody, 5)
	for _, test := range []struct {
		name       string
		owner      keyspace.Term
		wantReject bool
	}{
		{name: "same-body-loop-body", owner: body3},
		{name: "descendant-body", owner: body4},
		{name: "sibling-body", owner: body5, wantReject: true},
		{name: "enclosing-body", owner: body2, wantReject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := openLocalCellWriteFixture(t, test.owner)
			defer fixture.close()
			ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
				fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
			if test.wantReject {
				if err == nil {
					t.Fatal("SealPorts accepted a local Cell write across the ownership frontier")
				}
				return
			}
			if err != nil {
				t.Fatalf("SealPorts rejected a lexically valid local Cell write: %v", err)
			}
			if got, ok := ports.Finish(keyspace.MakeTerm(keyspace.FamilyAssign, 1)); !ok || got != keyspace.MakeTerm(keyspace.FamilyWrite, 1) {
				t.Fatalf("Finish(assign) = %v/%v, want Write1", got, ok)
			}
		})
	}
}

func TestPortsLocalCellWriteThroughFunctionChild(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 2, keyspace.FamilyCell: 2, keyspace.FamilyVararg: 1,
		keyspace.FamilyValues: 2, keyspace.FamilyFunction: 1,
		keyspace.FamilyReturn: 1,
		keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 1,
	}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{
			{Owner: body1, Fixed: authored.Range{End: 1}, Tail: keyspace.MakeTerm(keyspace.FamilyVararg, 1)},
			{Owner: body2, Fixed: authored.Range{Start: 1, End: 1}},
		}, Terms: []keyspace.Term{function}},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}},
			Varargs: []authored.Vararg{{Owner: body1, Cell: cell}},
			Assigns: []authored.Assign{{Owner: body2, Values: assignValues}},
			Writes:  []authored.Write{{Assign: assign, Target: cell}},
		},
		Functions: authored.FunctionsInput{
			Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
			Captures: []authored.Capture{{Inner: keyspace.MakeTerm(keyspace.FamilyCell, 2), Outer: cell}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body1, Values: values}}},
	}, 0)
	defer fixture.close()
	owner, body, _, ok := fixture.view.Functions().Get(function)
	if !ok || owner != body1 || body != body2 {
		t.Fatalf("Function = owner %v body %v/%v; want Body1 -> Body2", owner, body, ok)
	}
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("SealPorts rejected a captured outer Cell write from Function Body: %v", err)
	}
	if got, ok := ports.Finish(assign); !ok || got != keyspace.MakeTerm(keyspace.FamilyWrite, 1) {
		t.Fatalf("Finish(Function Assign) = %v/%v, want Write1", got, ok)
	}
}

func TestPortsAssignAndTableFormulas(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	fieldValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	rootValues := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	lensOne, lensTwo := keyspace.MakeTerm(keyspace.FamilyLensKey, 1), keyspace.MakeTerm(keyspace.FamilyLensKey, 2)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 7, keyspace.FamilyValues: 3,
		keyspace.FamilyLensKey: 2, keyspace.FamilyAssign: 1, keyspace.FamilyWrite: 3,
		keyspace.FamilyCell:  1,
		keyspace.FamilyTable: 1, keyspace.FamilyTableField: 1, keyspace.FamilyReturn: 1,
	}
	flow := authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{
			{Owner: body, Fixed: authored.Range{End: 1}},
			{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
			{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
		}, Terms: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2), table,
		}},
		Access: authored.AccessInput{Dynamic: []authored.DynamicLens{
			{Owner: body, Base: keyspace.MakeTerm(keyspace.FamilyNil, 3), Key: keyspace.MakeTerm(keyspace.FamilyNil, 4)},
			{Owner: body, Base: keyspace.MakeTerm(keyspace.FamilyNil, 5), Key: keyspace.MakeTerm(keyspace.FamilyNil, 6)},
		}},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: keyspace.Key(1)}},
			Assigns: []authored.Assign{{Owner: body, Values: values}},
			Writes: []authored.Write{
				{Assign: assign, Target: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
				{Assign: assign, Target: lensOne},
				{Assign: assign, Target: lensTwo},
			},
		},
		Tables: authored.TablesInput{
			Rows:   []authored.Table{{Owner: body, Fields: authored.Range{End: 1}}},
			Fields: []authored.Field{{Table: table, Values: fieldValues, Kind: kind.FieldKey, Key: keyspace.MakeTerm(keyspace.FamilyNil, 7)}},
			Order:  []keyspace.Term{field},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: rootValues}}},
	}
	fixture := openPortsFixture(t, counts, flow, 7)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ports.Entry(assign); !ok || got != keyspace.MakeTerm(keyspace.FamilyNil, 3) {
		t.Fatalf("Entry(assign) = %v, %v", got, ok)
	}
	if got, ok := ports.Finish(assign); !ok || got != keyspace.MakeTerm(keyspace.FamilyWrite, 1) {
		t.Fatalf("Finish(assign) = %v, %v", got, ok)
	}
	if got, ok := ports.Entry(table); !ok || got != table {
		t.Fatalf("Entry(table) = %v, %v", got, ok)
	}
	if got, ok := ports.Finish(table); !ok || got != field {
		t.Fatalf("Finish(table) = %v, %v", got, ok)
	}
}

func TestPortsMethodCallReceiverMetadata(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	receiver := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyKey: 1,
		keyspace.FamilyLensExact: 1, keyspace.FamilyRead: 1, keyspace.FamilyValues: 1,
		keyspace.FamilyCall: 1,
	}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Access: authored.AccessInput{Exact: []authored.ExactLens{{
			Owner: body, Base: receiver, Source: key, Kind: kind.FieldName,
		}}},
		Storage: authored.StorageInput{Reads: []authored.Read{{Owner: body, Source: lens}}},
		Calls:   []authored.Call{{Owner: body, Callee: read, Receiver: receiver, Actuals: actuals}},
	}, 1)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ports.Entry(call); !ok || got != receiver {
		t.Fatalf("Entry(method Call) = %v/%v, want receiver %v", got, ok, receiver)
	}
	if got, ok := ports.Finish(call); !ok || got != call {
		t.Fatalf("Finish(method Call) = %v/%v, want Call", got, ok)
	}
}

func TestPortsAllTableFieldKinds(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	rootValues := keyspace.MakeTerm(keyspace.FamilyValues, 5)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 4, keyspace.FamilyBool: 1,
		keyspace.FamilyInteger: 1, keyspace.FamilyKey: 2, keyspace.FamilyValues: 5,
		keyspace.FamilyTable: 1, keyspace.FamilyTableField: 4, keyspace.FamilyReturn: 1,
	}
	fields := make([]keyspace.Term, 4)
	for index := range fields {
		fields[index] = keyspace.MakeTerm(keyspace.FamilyTableField, uint32(index+1))
	}
	values := make([]keyspace.Term, 5)
	for index := range values {
		values[index] = keyspace.MakeTerm(keyspace.FamilyValues, uint32(index+1))
	}
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 5}},
			},
			Terms: []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2),
				keyspace.MakeTerm(keyspace.FamilyNil, 3), keyspace.MakeTerm(keyspace.FamilyNil, 4), table,
			},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 4}}},
			Fields: []authored.Field{
				{Table: table, Key: keyspace.MakeTerm(keyspace.FamilyKey, 1), Values: values[0], Kind: kind.FieldList},
				{Table: table, Key: keyspace.MakeTerm(keyspace.FamilyKey, 2), Values: values[1], Kind: kind.FieldName},
				{Table: table, Key: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Values: values[2], Kind: kind.FieldExact},
				{Table: table, Key: keyspace.MakeTerm(keyspace.FamilyBool, 1), Values: values[3], Kind: kind.FieldKey},
			},
			Order: fields,
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: rootValues}}},
	}
	fixture := openPortsFixture(t, counts, flow, 4)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	for ordinal := uint32(1); ordinal <= 4; ordinal++ {
		field := keyspace.MakeTerm(keyspace.FamilyTableField, ordinal)
		if entry, ok := ports.Entry(field); !ok || entry == 0 {
			t.Fatalf("Entry(TableField%d) = %v/%v", ordinal, entry, ok)
		}
		if finish, ok := ports.Finish(field); !ok || finish != field {
			t.Fatalf("Finish(TableField%d) = %v/%v", ordinal, finish, ok)
		}
	}
	if finish, ok := ports.Finish(table); !ok || finish != fields[3] {
		t.Fatalf("Finish(Table) = %v/%v, want %v", finish, ok, fields[3])
	}
}

func TestPortsOpenValuesCallAndSelectDisposition(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values, actuals := keyspace.MakeTerm(keyspace.FamilyValues, 2), keyspace.MakeTerm(keyspace.FamilyValues, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 3, keyspace.FamilyCell: 1, keyspace.FamilyVararg: 1,
		keyspace.FamilyValues: 2, keyspace.FamilySelect: 1, keyspace.FamilyCall: 1, keyspace.FamilyReturn: 1,
	}
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 1}, Tail: keyspace.MakeTerm(keyspace.FamilyVararg, 1)},
			},
			Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1)},
		},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Varargs: []authored.Vararg{{Owner: body, Cell: keyspace.MakeTerm(keyspace.FamilyCell, 1)}},
		},
		Calls: []authored.Call{{Owner: body, Callee: selectTerm, Actuals: actuals}},
		Operators: authored.OperatorsInput{Selects: []authored.Select{{
			Owner: body, Op: kind.SelectAnd,
			Left: keyspace.MakeTerm(keyspace.FamilyNil, 2), Right: keyspace.MakeTerm(keyspace.FamilyNil, 3),
		}}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
	fixture := openPortsFixture(t, counts, flow, 3)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := ports.Entry(values); !ok || entry != keyspace.MakeTerm(keyspace.FamilyVararg, 1) {
		t.Fatalf("Entry(open Values) = %v/%v", entry, ok)
	}
	if entry, ok := ports.Entry(call); !ok || entry != keyspace.MakeTerm(keyspace.FamilyNil, 2) {
		t.Fatalf("Entry(Call) = %v/%v", entry, ok)
	}
	if finish, ok := ports.Finish(call); !ok || finish != call {
		t.Fatalf("Finish(Call) = %v/%v", finish, ok)
	}
	if entry, ok := ports.Entry(selectTerm); !ok || entry != keyspace.MakeTerm(keyspace.FamilyNil, 2) {
		t.Fatalf("Entry(Select) = %v/%v", entry, ok)
	}
	if finish, ok := ports.Finish(selectTerm); !ok || finish != selectTerm {
		t.Fatalf("Finish(Select) = %v/%v", finish, ok)
	}
}

func TestPortsBranchAndRepeatLoopEntries(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	trueBody, falseBody, repeatBody := keyspace.MakeTerm(keyspace.FamilyBody, 2), keyspace.MakeTerm(keyspace.FamilyBody, 3), keyspace.MakeTerm(keyspace.FamilyBody, 4)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	loopControl := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 4, keyspace.FamilyNil: 2, keyspace.FamilyBranch: 1, keyspace.FamilyLoop: 1}
	fixture := openPortsFixture(t, counts, authored.Input{Control: authored.ControlInput{
		Branches: []authored.Branch{{Owner: body, Condition: nilTerm, WhenTrue: trueBody, WhenFalse: falseBody}},
		Loops:    []authored.Loop{{Owner: body, Body: repeatBody, Kind: kind.LoopRepeat, Control: loopControl}},
	}}, 2)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := ports.Entry(branch); !ok || got != nilTerm {
		t.Fatalf("Entry(branch) = %v, %v", got, ok)
	}
	if got, ok := ports.Entry(loop); !ok || got != repeatBody {
		t.Fatalf("Entry(repeat) = %v, %v", got, ok)
	}
}

func TestPortsAllLoopControlKinds(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 5, keyspace.FamilyNil: 2, keyspace.FamilyBool: 3,
		keyspace.FamilyCell: 2, keyspace.FamilyValues: 2, keyspace.FamilyLoop: 4,
	}
	loops := []authored.Loop{
		{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 2), Kind: kind.LoopWhile, Control: keyspace.MakeTerm(keyspace.FamilyNil, 1)},
		{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 3), Kind: kind.LoopRepeat, Control: keyspace.MakeTerm(keyspace.FamilyNil, 2)},
		{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 4), Kind: kind.LoopNumericFor, Control: keyspace.MakeTerm(keyspace.FamilyValues, 1), Cells: authored.Range{End: 1}},
		{Owner: entry, Body: keyspace.MakeTerm(keyspace.FamilyBody, 5), Kind: kind.LoopGenericFor, Control: keyspace.MakeTerm(keyspace.FamilyValues, 2), Cells: authored.Range{Start: 1, End: 2}},
	}
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: entry, Fixed: authored.Range{End: 2}},
				{Owner: entry, Fixed: authored.Range{Start: 2, End: 3}},
			},
			Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBool, 1), keyspace.MakeTerm(keyspace.FamilyBool, 2), keyspace.MakeTerm(keyspace.FamilyBool, 3)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: keyspace.MakeTerm(keyspace.FamilyBody, 4)},
			{Kind: authored.CellLocal, Body: keyspace.MakeTerm(keyspace.FamilyBody, 5)},
		}},
		Control: authored.ControlInput{Loops: loops, Cells: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyCell, 2),
		}},
	}
	fixture := openPortsFixture(t, counts, flow, 2)
	defer fixture.close()
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	want := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyNil, 1),
		keyspace.MakeTerm(keyspace.FamilyBody, 3),
		keyspace.MakeTerm(keyspace.FamilyBool, 1),
		keyspace.MakeTerm(keyspace.FamilyBool, 3),
	}
	for index, expected := range want {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		if entryTerm, ok := ports.Entry(loop); !ok || entryTerm != expected {
			t.Fatalf("Entry(loop %d) = %v/%v, want %v", index+1, entryTerm, ok, expected)
		}
		if finish, ok := ports.Finish(loop); !ok || finish != loop {
			t.Fatalf("Finish(loop %d) = %v/%v", index+1, finish, ok)
		}
	}
}

func TestPortsLoopOwnerFrontierRejectsForeignControlClaims(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	whileBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	repeatBody := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	numericBody := keyspace.MakeTerm(keyspace.FamilyBody, 4)
	genericBody := keyspace.MakeTerm(keyspace.FamilyBody, 5)
	whileLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	repeatLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	numericLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 3)
	genericLoop := keyspace.MakeTerm(keyspace.FamilyLoop, 4)
	whileControl := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	repeatControl := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	numericControl := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	genericControl := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 5, keyspace.FamilyNil: 2, keyspace.FamilyBool: 3,
		keyspace.FamilyCell: 2, keyspace.FamilyValues: 2, keyspace.FamilyLoop: 4,
	}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: entry, Fixed: authored.Range{End: 2}},
				{Owner: entry, Fixed: authored.Range{Start: 2, End: 3}},
			},
			Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBool, 1), keyspace.MakeTerm(keyspace.FamilyBool, 2), keyspace.MakeTerm(keyspace.FamilyBool, 3)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: numericBody},
			{Kind: authored.CellLocal, Body: genericBody},
		}},
		Control: authored.ControlInput{
			Loops: []authored.Loop{
				{Owner: entry, Body: whileBody, Kind: kind.LoopWhile, Control: whileControl},
				{Owner: entry, Body: repeatBody, Kind: kind.LoopRepeat, Control: repeatControl},
				{Owner: entry, Body: numericBody, Kind: kind.LoopNumericFor, Control: numericControl, Cells: authored.Range{End: 1}},
				{Owner: entry, Body: genericBody, Kind: kind.LoopGenericFor, Control: genericControl, Cells: authored.Range{Start: 1, End: 2}},
			},
			Cells: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyCell, 2)},
		},
	}, 2)
	defer fixture.close()
	portsView := fixture.view
	forest := fixture.forest

	for _, test := range []struct {
		name  string
		owner keyspace.Term
		term  keyspace.Term
	}{
		{name: "while-control-cannot-claim-child", owner: whileBody, term: whileControl},
		{name: "numeric-control-cannot-claim-child", owner: numericBody, term: numericControl},
		{name: "generic-control-cannot-claim-child", owner: genericBody, term: genericControl},
		{name: "repeat-control-cannot-claim-enclosing", owner: entry, term: repeatControl},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := requireOwner(portsView, forest, test.owner, test.term); err == nil {
				t.Fatalf("requireOwner(%v, %v) accepted a foreign control frontier", test.owner, test.term)
			}
		})
	}
	if err := requireOwner(portsView, forest, repeatBody, repeatControl); err != nil {
		t.Fatalf("Repeat Body control owner rejected: %v", err)
	}

	for _, test := range []struct {
		name string
		term keyspace.Term
	}{
		{name: "while-control", term: whileControl},
		{name: "while-loop", term: whileLoop},
		{name: "repeat-loop", term: repeatLoop},
		{name: "numeric-loop", term: numericLoop},
		{name: "generic-loop", term: genericLoop},
		{name: "enclosing-body", term: entry},
	} {
		t.Run("repeat-frontier/"+test.name, func(t *testing.T) {
			if err := requireOwner(portsView, forest, repeatBody, test.term); err == nil {
				t.Fatalf("requireOwner admitted unrelated term %v to the Repeat Body", test.term)
			}
		})
	}
}

func TestPortsDoNotAllocateStaticOnlyPlanes(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyTypePrimitive: 2048,
	}
	ports := newPorts(func() [keyspace.FamilyCount]int {
		var result [keyspace.FamilyCount]int
		for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
			result[family] = int(counts[family])
		}
		return result
	}(), func() keyspace.ContentID {
		var id keyspace.ContentID
		id[0] = 1
		return id
	}(), func() keyspace.ContentID {
		var id keyspace.ContentID
		id[0] = 2
		return id
	}(), func() keyspace.ContentID {
		var id keyspace.ContentID
		id[0] = 3
		return id
	}(), func() keyspace.ContentID {
		var id keyspace.ContentID
		id[0] = 4
		return id
	}())
	if len(ports.entry[keyspace.FamilyTypePrimitive]) != 0 || len(ports.finish[keyspace.FamilyTypePrimitive]) != 0 {
		t.Fatal("static-only family retained evaluation planes")
	}
	if _, ok := ports.Entry(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("static-only term received Entry")
	}
	if _, ok := ports.Finish(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("static-only term received Finish")
	}
}

func TestPortsRejectExpiredSourceAndRetainNoAuthority(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1}
	fixture := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilTerm}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 1)
	ports, err := SealPorts(fixture.identity, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SealPorts(source.Identity{}, fixture.view, fixture.forest,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("expired Source identity was accepted")
	}
	fixture.close()
	if entry, ok := ports.Entry(nilTerm); !ok || entry != nilTerm {
		t.Fatalf("Entry after Source expiry = %v, %v", entry, ok)
	}
}

func TestPortsRejectForeignEqualCardinalityForest(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 2,
		keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1,
	}
	first := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}}, Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2)}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 2)
	defer first.close()
	second := openPortsFixture(t, counts, authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}}, Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyNil, 2), keyspace.MakeTerm(keyspace.FamilyNil, 1)}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}, 2)
	defer second.close()
	if first.identity.ContentID() != second.identity.ContentID() {
		t.Fatal("fixture source identities unexpectedly differ")
	}
	if first.view.Cold().ContentID() == second.view.Cold().ContentID() {
		t.Fatal("fixture Flow identities unexpectedly match")
	}
	if _, err := SealPorts(second.identity, second.view, first.forest,
		second.staticFinalize.View().ContentID(), second.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("equal-cardinality foreign containment proof was accepted")
	}
}

func TestPortsMatchesRejectsUnavailableAndEqualDenominatorForeignOwners(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTwo := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 2,
		keyspace.FamilyValues: 1, keyspace.FamilyReturn: 1,
	}
	input := func(terms []keyspace.Term) authored.Input {
		return authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 2}}},
				Terms: terms,
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
		}
	}
	first := openPortsFixture(t, counts, input([]keyspace.Term{nilOne, nilTwo}), 2)
	defer first.close()
	foreignFlow := openPortsFixture(t, counts, input([]keyspace.Term{nilTwo, nilOne}), 2)
	defer foreignFlow.close()
	firstPorts, err := SealPorts(first.identity, first.view, first.forest,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("SealPorts(first): %v", err)
	}
	foreignPorts, err := SealPorts(foreignFlow.identity, foreignFlow.view, foreignFlow.forest,
		foreignFlow.staticFinalize.View().ContentID(), foreignFlow.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("SealPorts(foreignFlow): %v", err)
	}
	sourceID := first.identity.ContentID()
	flowID := first.view.Cold().ContentID()
	staticID := first.staticFinalize.View().ContentID()
	moduleID := first.moduleFinalize.View().ContentID()
	if !Matches(firstPorts, sourceID, flowID, staticID, moduleID) {
		t.Fatal("sealed Ports did not match its exact owners")
	}
	if first.identity.TermCount() != foreignFlow.identity.TermCount() ||
		first.view.Values().Count() != foreignFlow.view.Values().Count() ||
		flowID == foreignFlow.view.Cold().ContentID() {
		t.Fatal("foreign fixture did not preserve denominator while changing Flow identity")
	}
	if Matches(nil, sourceID, flowID, staticID, moduleID) || Matches(firstPorts, keyspace.ContentID{}, flowID, staticID, moduleID) ||
		Matches(firstPorts, sourceID, keyspace.ContentID{}, staticID, moduleID) ||
		Matches(firstPorts, sourceID, flowID, keyspace.ContentID{}, moduleID) || Matches(firstPorts, sourceID, flowID, staticID, keyspace.ContentID{}) {
		t.Fatal("Ports provenance accepted nil or unavailable owner identity")
	}
	foreignSourceID := sourceID
	foreignSourceID[0]++
	foreignFlowID := flowID
	foreignFlowID[0]++
	foreignStaticID := staticID
	foreignStaticID[0]++
	foreignModuleID := moduleID
	foreignModuleID[0]++
	if Matches(firstPorts, foreignSourceID, flowID, staticID, moduleID) || Matches(firstPorts, sourceID, foreignFlowID, staticID, moduleID) ||
		Matches(firstPorts, sourceID, flowID, foreignStaticID, moduleID) || Matches(firstPorts, sourceID, flowID, staticID, foreignModuleID) ||
		Matches(foreignPorts, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Ports provenance accepted a foreign equal-denominator owner")
	}
	var zero Ports
	if Matches(&zero, sourceID, flowID, staticID, moduleID) {
		t.Fatal("zero Ports bypassed provenance fence")
	}
	if entry, ok := zero.Entry(nilOne); ok || entry != 0 {
		t.Fatalf("zero-ID Entry = %v/%v, want 0/false", entry, ok)
	}
	if finish, ok := zero.Finish(nilOne); ok || finish != 0 {
		t.Fatalf("zero-ID Finish = %v/%v, want 0/false", finish, ok)
	}
}
