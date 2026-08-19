package flow

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
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

// These laws exercise actual Source/authored owner views and one complete
// quartet-gated seal. They do not manufacture proof results or a module Entry
// to make an assertion green; malformed owner and occurrence cases fail
// through the same private projections used by assembly.

func TestModuleEntryRetainsExecutableReturnSlotsAndDirectFunction(t *testing.T) {
	fixture := openModuleEntryFunctionFixture(t)
	input, err := sealModuleEntry(
		fixture.source, fixture.flow, fixture.module, fixture.bodies,
		fixture.executable, fixture.directFunctions, fixture.staticID, fixture.entry,
	)
	if err != nil {
		t.Fatalf("sealModuleEntry: %v", err)
	}
	if len(input.Entry.ReturnTerms) != 1 || input.Entry.ReturnTerms[0] != fixture.returned {
		t.Fatalf("Return terms = %#v, want chunk Return", input.Entry.ReturnTerms)
	}
	if len(input.Entry.Roots) != 1 || input.Entry.Roots[0] != fixture.function {
		t.Fatalf("fixed Return roots = %#v, want direct Function", input.Entry.Roots)
	}
	if len(input.Entry.RootCells) != 1 || input.Entry.RootCells[0] != 0 {
		t.Fatalf("fixed Return root Cells = %#v, want no fabricated Cell", input.Entry.RootCells)
	}
	if input.Entry.ReturnIndex[fixture.returnedOrdinal] != 1 || input.Entry.RootRanges[fixture.returnedOrdinal] != (imports.EntryRange{Start: 0, End: 1}) {
		t.Fatalf("Return ordinal/range = %d/%#v, want one fixed slot", input.Entry.ReturnIndex[fixture.returnedOrdinal], input.Entry.RootRanges[fixture.returnedOrdinal])
	}
	component, err := fixture.moduleFinalizer.Commit(input)
	if err != nil {
		t.Fatalf("Module commit: %v", err)
	}
	entry := component.View().Entry()
	if got, ok := entry.RootFunction(fixture.returned, 0); !ok || got != fixture.function {
		t.Fatalf("committed RootFunction = %v/%v, want direct Function", got, ok)
	}
}

type moduleEntryFunctionFixture struct {
	source          source.View
	flow            authored.View
	module          imports.View
	bodies          *body.Result
	executable      *executable.Result
	directFunctions *directfunction.Result
	staticID        identity.ContentID
	entry           keyspace.Term
	returned        keyspace.Term
	returnedOrdinal uint32
	function        keyspace.Term
	moduleFinalizer imports.Finalizer
}

func openModuleEntryFunctionFixture(t *testing.T) *moduleEntryFunctionFixture {
	t.Helper()
	entryBody := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	counts := familyCounts(keyspace.FamilyBody, keyspace.FamilyFunction, keyspace.FamilyReturn, keyspace.FamilyValues)
	counts[keyspace.FamilyBody] = 2

	name := "module-entry-function.lua"
	sourceDraft, err := source.Build(source.Input{
		Name:     name,
		Families: familySpansNamed(name, counts),
		Bodies: []source.BodySource{
			{Body: entryBody, Terms: []keyspace.Term{returned}},
			{Body: functionBody},
		},
		Functions: []source.FunctionFormals{{Function: function}},
	})
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalizer.Preimage()

	staticInput := static.Input{}
	staticInput.Counts[keyspace.FamilyBody] = counts[keyspace.FamilyBody]
	staticInput.Counts[keyspace.FamilyValues] = counts[keyspace.FamilyValues]
	staticInput.Counts[keyspace.FamilyFunction] = counts[keyspace.FamilyFunction]
	staticInput.Contracts.Function = []staticcontracts.FunctionContract{{}}
	staticInput.Counts[keyspace.FamilyFunction] = uint32(len(staticInput.Contracts.Function))
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}

	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: entryBody, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{function},
		},
		Functions: authored.FunctionsInput{
			Rows: []authored.Function{{Owner: entryBody, Body: functionBody}},
		},
		Control: authored.ControlInput{
			Returns: []authored.Return{{Owner: entryBody, Values: values}},
		},
	})
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("Finalizer: %v", err)
	}
	flowView := flowFinalizer.View()
	entry := entryBody
	bodies, err := body.Seal(preimage, flowView, staticFinalizer.View(), entry)
	if err != nil {
		_ = flowFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		_ = flowFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	moduleView := moduleFinalizer.View()
	forest, _, err := containment.Prove(preimage, staticFinalizer.View(), flowView, bodies, bindingResult, moduleView, entry)
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = flowFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("containment.Prove: %v", err)
	}
	staticID, moduleID := staticFinalizer.View().ContentID(), moduleView.ContentID()
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticID, moduleID)
	if err != nil {
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticID, moduleID)
	if err != nil {
		t.Fatalf("outcome.Seal: %v", err)
	}
	indexInput, err := position.Seal(preimage, flowView, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, issuance, err := sourceFinalizer.CommitWithSemanticPathIssuance(indexInput)
	if err != nil {
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlGraph, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, entry, staticID, moduleID)
	if err != nil {
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	paths, err := semanticpath.Seal(issuance, sourceView.CellRoles(), sourceView, flowView, bodies, bindingResult, forest, outcomes,
		flowView.Cold().ContentID(), staticID, moduleID)
	if err != nil {
		t.Fatalf("semanticpath.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, forest, controlGraph, staticID, moduleID, paths)
	if err != nil {
		t.Fatalf("executable.Seal: %v", err)
	}
	directFunctions, err := directfunction.Seal(sourceView, flowView, bodies, bindingResult, forest, controlGraph, executableResult, staticID, moduleID)
	if err != nil {
		t.Fatalf("directfunction.Seal: %v", err)
	}
	t.Cleanup(func() {
		_ = moduleFinalizer.Abort()
		_ = flowFinalizer.Abort()
		_ = staticFinalizer.Abort()
	})
	return &moduleEntryFunctionFixture{
		source: sourceView, flow: flowView, module: moduleView, bodies: bodies,
		executable: executableResult, directFunctions: directFunctions,
		staticID: staticID, entry: entry, returned: returned, returnedOrdinal: keyspace.TermOrdinal(returned), function: function,
		moduleFinalizer: moduleFinalizer,
	}
}

func TestModuleEntryTableSelectionLastWriteAndDynamicFence(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	keys := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyKey, 1), keyspace.MakeTerm(keyspace.FamilyKey, 2)}
	strings := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyString, 1), keyspace.MakeTerm(keyspace.FamilyString, 2)}
	valueTerms := []keyspace.Term{strings[0], strings[1], strings[0], strings[1]}
	fields := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTableField, 1),
		keyspace.MakeTerm(keyspace.FamilyTableField, 2),
		keyspace.MakeTerm(keyspace.FamilyTableField, 3),
		keyspace.MakeTerm(keyspace.FamilyTableField, 4),
	}
	values := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyValues, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 3),
		keyspace.MakeTerm(keyspace.FamilyValues, 4),
	}
	counts := familyCounts(keyspace.FamilyBody, keyspace.FamilyString, keyspace.FamilyValues, keyspace.FamilyTable, keyspace.FamilyTableField, keyspace.FamilyKey)
	counts[keyspace.FamilyString] = 2
	counts[keyspace.FamilyValues] = 4
	counts[keyspace.FamilyTableField] = 4
	counts[keyspace.FamilyKey] = 2
	sourceView := moduleEntrySource(t, source.Input{
		Name:     "table.lua",
		Families: familySpansNamed("table.lua", counts),
		String:   []source.StringLiteral{{Owner: body, Value: "a"}, {Owner: body, Value: "b"}},
		ExactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "a"},
			{Kind: keyspace.LiteralString, String: "b"},
		},
		Keys:   []source.KeyInput{source.NameKey(body, "a"), source.NameKey(body, "b")},
		Bodies: []source.BodySource{{Body: body}},
	})
	flowView := moduleEntryFlow(t, authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
			},
			Terms: valueTerms,
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 4}}},
			Fields: []authored.Field{
				{Table: table, Key: keys[0], Values: values[0], Kind: kind.FieldName},
				{Table: table, Key: strings[0], Values: values[1], Kind: kind.FieldKey},
				{Table: table, Key: keys[1], Values: values[2], Kind: kind.FieldName},
				{Table: table, Key: keys[0], Values: values[3], Kind: kind.FieldName},
			},
			Order: fields,
		},
	})

	first, err := selectEntryFields(sourceView, flowView, table)
	if err != nil {
		t.Fatalf("selectEntryFields: %v", err)
	}
	second, err := selectEntryFields(sourceView, flowView, table)
	if err != nil {
		t.Fatalf("second selectEntryFields: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("selection changed across identical queries: %#v != %#v", first, second)
	}
	if len(first) != 2 || first[0].field != fields[2] || first[1].field != fields[3] {
		t.Fatalf("selected fields = %#v, want final b/a fields after dynamic fence", first)
	}
	if first[0].key == 0 || first[1].key == 0 || first[0].key == first[1].key {
		t.Fatalf("selected fields did not retain distinct Source exact keys: %#v", first)
	}
	scratch, err := newEntryKeyScratch(sourceView)
	if err != nil {
		t.Fatalf("newEntryKeyScratch: %v", err)
	}
	plane := &scratch.marks[0]
	third, err := selectEntryFieldsWithScratch(sourceView, flowView, table, &scratch)
	if err != nil {
		t.Fatalf("first shared-scratch selection: %v", err)
	}
	fourth, err := selectEntryFieldsWithScratch(sourceView, flowView, table, &scratch)
	if err != nil {
		t.Fatalf("second shared-scratch selection: %v", err)
	}
	if scratch.epoch != 2 || plane != &scratch.marks[0] || !reflect.DeepEqual(third, fourth) {
		t.Fatalf("shared exact-key plane was replaced or changed truth: epoch=%d third=%#v fourth=%#v", scratch.epoch, third, fourth)
	}
}

func TestModuleEntryTableSelectionIsStableAcrossCopiedViews(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	stringTerm := keyspace.MakeTerm(keyspace.FamilyString, 1)
	valuesTerm := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	counts := familyCounts(keyspace.FamilyBody, keyspace.FamilyString, keyspace.FamilyValues, keyspace.FamilyTable, keyspace.FamilyTableField, keyspace.FamilyKey)
	sourceInput := source.Input{
		Name:       "copied-table.lua",
		Families:   familySpansNamed("copied-table.lua", counts),
		String:     []source.StringLiteral{{Owner: body, Value: "a"}},
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "a"}},
		Keys:       []source.KeyInput{source.NameKey(body, "a")},
		Bodies:     []source.BodySource{{Body: body}},
	}
	sourceView := moduleEntrySource(t, sourceInput)
	flowInput := authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{stringTerm},
		},
		Tables: authored.TablesInput{
			Rows:   []authored.Table{{Owner: body, Fields: authored.Range{End: 1}}},
			Fields: []authored.Field{{Table: table, Key: key, Values: valuesTerm, Kind: kind.FieldName}},
			Order:  []keyspace.Term{field},
		},
	}
	first := moduleEntryFlow(t, flowInput)
	second := moduleEntryFlow(t, flowInput)
	left, err := selectEntryFields(sourceView, first, table)
	if err != nil {
		t.Fatalf("first copied view selection: %v", err)
	}
	right, err := selectEntryFields(sourceView, second, table)
	if err != nil {
		t.Fatalf("second copied view selection: %v", err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("copied authored views changed selection: %#v != %#v", left, right)
	}
}

func TestModuleEntryStringMemberBoundaryRejectsNumericUnaryAndDynamicKeys(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	stringTerm := keyspace.MakeTerm(keyspace.FamilyString, 1)
	unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	nameKey := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	values := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyValues, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 3),
		keyspace.MakeTerm(keyspace.FamilyValues, 4),
		keyspace.MakeTerm(keyspace.FamilyValues, 5),
	}
	fields := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTableField, 1),
		keyspace.MakeTerm(keyspace.FamilyTableField, 2),
		keyspace.MakeTerm(keyspace.FamilyTableField, 3),
		keyspace.MakeTerm(keyspace.FamilyTableField, 4),
		keyspace.MakeTerm(keyspace.FamilyTableField, 5),
	}
	counts := familyCounts(
		keyspace.FamilyBody, keyspace.FamilyInteger, keyspace.FamilyString,
		keyspace.FamilyUnary, keyspace.FamilyTable, keyspace.FamilyKey,
	)
	counts[keyspace.FamilyValues] = 5
	counts[keyspace.FamilyTableField] = 5
	sourceView := moduleEntrySource(t, source.Input{
		Name:     "string-member-boundary.lua",
		Families: familySpansNamed("string-member-boundary.lua", counts),
		Integer:  []source.IntegerLiteral{{Owner: body, Value: 7}},
		String:   []source.StringLiteral{{Owner: body, Value: "bracket"}},
		ExactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralInteger, Integer: 7},
			{Kind: keyspace.LiteralInteger, Integer: -7},
			{Kind: keyspace.LiteralString, String: "bracket"},
			{Kind: keyspace.LiteralString, String: "kept"},
		},
		Keys:   []source.KeyInput{source.NameKey(body, "kept")},
		Bodies: []source.BodySource{{Body: body}},
	})
	flowView := moduleEntryFlow(t, authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 5}},
			},
			Terms: []keyspace.Term{stringTerm, stringTerm, stringTerm, stringTerm, stringTerm},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 5}}},
			Fields: []authored.Field{
				{Table: table, Key: integer, Values: values[0], Kind: kind.FieldExact},
				{Table: table, Key: unary, Values: values[1], Kind: kind.FieldExact},
				{Table: table, Key: stringTerm, Values: values[2], Kind: kind.FieldKey},
				{Table: table, Key: nameKey, Values: values[3], Kind: kind.FieldName},
				{Table: table, Key: stringTerm, Values: values[4], Kind: kind.FieldExact},
			},
			Order: fields,
		},
		Operators: authored.OperatorsInput{
			Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: integer}},
		},
	})

	if _, ok := entryFieldKey(sourceView, kind.FieldExact, integer); ok {
		t.Fatal("numeric exact key crossed the string-only Module entry boundary")
	}
	if _, ok := entryFieldKey(sourceView, kind.FieldExact, unary); ok {
		t.Fatal("UnaryNeg exact key crossed the string-only Module entry boundary")
	}
	if _, ok := entryFieldKey(sourceView, kind.FieldKey, stringTerm); ok {
		t.Fatal("dynamic bracket key crossed the exact member boundary")
	}
	stringKey, ok := entryFieldKey(sourceView, kind.FieldExact, stringTerm)
	if !ok || stringKey == 0 {
		t.Fatal("direct Source string occurrence was not resolved through Source Keys")
	}
	selected, err := selectEntryFields(sourceView, flowView, table)
	if err != nil {
		t.Fatalf("selectEntryFields: %v", err)
	}
	if len(selected) != 2 || selected[0].field != fields[3] || selected[1].field != fields[4] || selected[1].key != stringKey {
		t.Fatalf("selected fields = %#v, want named and direct-string members only", selected)
	}
}

func TestModuleEntryReadCellKeepsOnlyImmediateCellWitness(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	counts := familyCounts(keyspace.FamilyBody, keyspace.FamilyCell, keyspace.FamilyRead)
	flowView := moduleEntryFlow(t, authored.Input{
		Counts: counts,
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
			Reads: []authored.Read{{Owner: body, Source: cell}},
		},
	})
	if got := entryDirectCell(flowView, read); got != cell {
		t.Fatalf("Read direct Cell = %v, want %v", got, cell)
	}
	if got := entryDirectCell(flowView, cell); got != 0 {
		t.Fatalf("non-Read direct Cell = %v, want absent", got)
	}
}

func familyCounts(families ...keyspace.Family) [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	for _, family := range families {
		counts[family]++
	}
	return counts
}

func familySpansNamed(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	rows := make([]source.FamilySpans, 0, int(keyspace.FamilyCount)-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		rows = append(rows, source.FamilySpans{Family: family, Spans: spans})
	}
	return rows
}

func moduleEntrySource(t *testing.T, input source.Input) source.View {
	t.Helper()
	draft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := finalizer.Preimage()
	identity := preimage.Identity()
	bodyCount := identity.FamilyCount(keyspace.FamilyBody)
	bodies := make([]source.BodyRoots, bodyCount)
	for index := range bodies {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
		// The focused fixtures use one chunk Body. The malformed-owner fixture
		// adds one reachable child so Source can accept both owner rows.
		var parent keyspace.Term
		if index > 0 && bodyCount > 1 {
			parent = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}
		bodies[index] = source.BodyRoots{Body: body, Parent: parent}
	}
	component, err := finalizer.Commit(source.IndexInput{
		SourceID: identity.ContentID(),
		Bodies:   bodies,
		Entry:    keyspace.MakeTerm(keyspace.FamilyBody, 1),
	})
	if err != nil {
		t.Fatalf("source.Commit: %v", err)
	}
	return component.View()
}

func moduleEntryFlow(t *testing.T, input authored.Input) authored.View {
	t.Helper()
	draft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	view, err := finalizer.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return view
}

func moduleEntryModule(t *testing.T, input imports.Input) imports.View {
	t.Helper()
	draft, err := imports.Build(input)
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	return finalizer.View()
}
