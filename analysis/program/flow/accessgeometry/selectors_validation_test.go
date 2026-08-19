package accessgeometry

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// TestImportAliasRequiresExactCrossProof exercises the full import-alias
// bridge: the Module row points at a plain same-owner require Call, Binding
// proves the local alias's unique Bind host, and Source proves that Bind's
// order and first Values position contain exactly that Call.

func proveImportAlias(t *testing.T, openTail bool) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	requireCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	aliasCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	keyRequire := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	valuesBind := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	valuesArgs := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	readRequire := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readAlias := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	importTerm := keyspace.MakeTerm(keyspace.FamilyImport, 1)

	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{body, requireCell, aliasCell, keyRequire, bind, valuesBind, valuesArgs, readRequire, readAlias, call, importTerm} {
		counts[keyspace.TermFamily(term)]++
	}

	input := source.Input{
		Name:       "accessgeometry-import.lua",
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "require"}},
		Keys:       []source.KeyInput{source.NameKey(body, "require")},
		Binds:      []source.BindCells{{Bind: bind, Cells: []keyspace.Term{aliasCell}}},
		Bodies:     []source.BodySource{{Body: body, Terms: []keyspace.Term{bind, call}}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer func() { _ = sourceFinalizer.Abort() }()

	bindValues := authored.Value{Owner: body}
	if openTail {
		bindValues.Tail = call
	} else {
		bindValues.Fixed = authored.Range{End: 1}
	}
	actualValues := authored.Value{Owner: body, Fixed: authored.Range{Start: 1, End: 1}}
	if openTail {
		// The open row has no fixed member and therefore does not advance the
		// shared Values member cursor used by the second (call-argument) row.
		actualValues.Fixed = authored.Range{End: 1}
	}
	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{bindValues, actualValues},
			Terms: []keyspace.Term{call},
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{
				{Kind: authored.CellGlobal, Key: 1},
				{Kind: authored.CellLocal, Body: body},
			},
			Reads: []authored.Read{
				{Owner: body, Source: requireCell},
				{Owner: body, Source: aliasCell},
			},
			Binds: []authored.Bind{{Owner: body, Values: valuesBind}},
		},
		Calls: []authored.Call{{Owner: body, Callee: readRequire, Actuals: valuesArgs}},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{Imports: []imports.Import{{Term: importTerm, Call: call, Alias: aliasCell, Request: keyspace.MakeTerm(keyspace.FamilyString, 1)}}})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()

	flowView := flowFinalizer.View()
	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	bodyResult, err := flowbody.Seal(sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := sealSelectors(sourceFinalizer.Preimage(), flowView, bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	root, depth, ok := result.ExactReads().Get(readAlias)
	if !ok || root != aliasCell || depth != 0 {
		t.Fatalf("import alias selection = %v/%d/%v, want %v/0/true", root, depth, ok, aliasCell)
	}
	read, form, ok := result.DirectCalls().Get(call)
	if !ok || read != readRequire || form != selectorCallPlain {
		t.Fatalf("require call = %v/%v/%v, want %v/plain/true", read, form, ok, readRequire)
	}
}

func TestSealRejectsUnaryDenominatorMismatch(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	var sourceCounts [keyspace.FamilyCount]uint32
	sourceCounts[keyspace.FamilyBody] = 1
	sourceCounts[keyspace.FamilyUnary] = 1

	sourceInput := source.Input{
		Name:   "accessgeometry-unary-denominator.lua",
		Bodies: []source.BodySource{{Body: body}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, sourceCounts[family])
		for index := range spans {
			spans[index] = source.Span{
				File: sourceInput.Name, StartLine: uint32(index + 1), StartCol: 1,
				EndLine: uint32(index + 1), EndCol: 1,
			}
		}
		sourceInput.Families = append(sourceInput.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer func() { _ = sourceFinalizer.Abort() }()

	flowCounts := sourceCounts
	flowCounts[keyspace.FamilyUnary] = 0
	flowDraft, err := authored.Build(authored.Input{Counts: flowCounts})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()

	flowView := flowFinalizer.View()
	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	bodyResult, bodyErr := flowbody.Seal(sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	if bodyErr != nil {
		t.Fatalf("body.Seal: %v", bodyErr)
	}
	_, err = sealSelectors(sourceFinalizer.Preimage(), flowView, bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err == nil || !strings.Contains(err.Error(), "authored family denominator mismatch") {
		t.Fatalf("Seal error = %v, want Unary denominator mismatch rejection", err)
	}
}
