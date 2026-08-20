package flow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

func TestAssembleNilDraftFailsClosed(t *testing.T) {
	sourceFinalizer, staticComponent, staticView, moduleFinalizer, unusedDraft := emptyAssemblyOwners(t, "nil-draft.lua")
	preimage := sourceFinalizer.Preimage()
	moduleView := moduleFinalizer.View()
	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, nil, 0)
	if err == nil || assembly != nil {
		t.Fatalf("Assemble(nil) = %#v, %v; want nil assembly and error", assembly, err)
	}
	if !preimage.Identity().ContentID().Available() || !staticView.ContentID().Available() || !moduleView.ContentID().Available() {
		t.Fatal("nil Flow Draft claim failure consumed a sibling owner")
	}
	abortClaimedSiblingOwners(t, sourceFinalizer, moduleFinalizer)
	unusedFinalizer, err := unusedDraft.claim()
	if err != nil {
		t.Fatalf("unused Flow Draft claim: %v", err)
	}
	if err := unusedFinalizer.Abort(); err != nil {
		t.Fatalf("unused Flow Finalizer abort: %v", err)
	}
}

func TestAssembleAlreadyClaimedFlowLeavesSiblingOwnersLive(t *testing.T) {
	sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft := emptyAssemblyOwners(t, "claimed-draft.lua")
	preimage := sourceFinalizer.Preimage()
	moduleView := moduleFinalizer.View()
	claimedFlow, err := draft.claim()
	if err != nil {
		t.Fatalf("Flow claim: %v", err)
	}
	flowView := claimedFlow.View()

	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err == nil || assembly != nil {
		t.Fatalf("Assemble(already claimed) = %#v, %v; want nil assembly and error", assembly, err)
	}
	if !preimage.Identity().ContentID().Available() || !staticView.ContentID().Available() ||
		!moduleView.ContentID().Available() || !flowView.Cold().ContentID().Available() {
		t.Fatal("failed Flow claim consumed an owner held by the winning invocation")
	}
	abortClaimedSiblingOwners(t, sourceFinalizer, moduleFinalizer)
	if err := claimedFlow.Abort(); err != nil {
		t.Fatalf("claimed Flow abort: %v", err)
	}
}

func TestAssembleSuccessfulQuartetPublishesStaticReceipt(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft := emptyAssemblyOwners(t, "successful-quartet.lua")
	preimage := sourceFinalizer.Preimage()
	moduleView := moduleFinalizer.View()
	copiedDraft := *draft

	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, &copiedDraft, entry)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if assembly == nil {
		t.Fatal("Assemble returned no transfer token")
	}
	copiedAssembly := *assembly
	sourceComponent, flowComponent, staticComponent, moduleComponent, takeErr := assembly.Take()
	if takeErr != nil || sourceComponent == nil || flowComponent == nil || staticComponent == nil || moduleComponent == nil {
		t.Fatalf("Assembly.Take = %p/%p/%p/%p, %v; want complete quartet", sourceComponent, flowComponent, staticComponent, moduleComponent, takeErr)
	}
	provenance := flowComponent.View().Provenance()
	if !flowComponent.View().AccessGeometry().Available() || !flowComponent.View().BinaryPrimitives().Available() {
		t.Fatal("valid zero-row projections reported unavailable")
	}
	if provenance.Source != sourceComponent.View().Identity().ContentID() ||
		provenance.Flow != flowComponent.ContentID() ||
		provenance.Static != staticComponent.ContentID() ||
		provenance.Module != moduleComponent.View().ContentID() {
		t.Fatalf("Flow provenance = %#v; want exact committed quartet", provenance)
	}
	secondSource, secondFlow, secondStatic, secondModule, secondErr := copiedAssembly.Take()
	if secondErr == nil || secondSource != nil || secondFlow != nil || secondStatic != nil || secondModule != nil {
		t.Fatalf("copied Assembly.Take = %p/%p/%p/%p, %v; want all nil and error", secondSource, secondFlow, secondStatic, secondModule, secondErr)
	}
	if preimage.Identity().ContentID().Available() || !staticView.ContentID().Available() || moduleView.ContentID().Available() {
		t.Fatal("successful Assemble left a consumed construction owner View readable or lost Static view")
	}
	if repeated, repeatedErr := Assemble(source.Finalizer{}, staticComponent, staticView, imports.Finalizer{}, draft, entry); repeatedErr == nil || repeated != nil {
		t.Fatalf("copied Draft reopened Flow: assembly=%#v err=%v", repeated, repeatedErr)
	}
}

func TestAssemblePublishesAccessAndBinaryProjections(t *testing.T) {
	assembly := openProjectionAssembly(t, "projection-quartet.lua")
	sourceComponent, flowComponent, _, _, err := assembly.Take()
	if err != nil {
		t.Fatalf("Assembly.Take: %v", err)
	}
	view := flowComponent.View()
	geometry := view.AccessGeometry()
	if !geometry.Available() {
		t.Fatal("valid assembled AccessGeometry reported unavailable")
	}
	if geometry.TableFields().Count() != 1 {
		t.Fatalf("TableField count = %d, want one retained access-geometry row", geometry.TableFields().Count())
	}
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	if got, ok := geometry.TableFields().At(0); !ok || got != field {
		t.Fatalf("TableFields.At(0) = %v/%v, want %v", got, ok, field)
	}
	_, keyTerm, _, fieldKind, ok := view.Authored().Fields().Get(field)
	if !ok || fieldKind != kind.FieldName {
		t.Fatalf("authored TableField(%v) = key=%v kind=%v/%v, want a FieldName", field, keyTerm, fieldKind, ok)
	}
	owner, spelling, expectedKey, ok := sourceComponent.View().Keys().Name(keyTerm)
	if !ok || owner != keyspace.MakeTerm(keyspace.FamilyBody, 1) || spelling != "field" || expectedKey == 0 {
		t.Fatalf("Source Keys.Name(%v) = owner=%v spelling=%q key=%v/%v, want Body/field/exact key", keyTerm, owner, spelling, expectedKey, ok)
	}
	if got, ok := geometry.TableFields().Get(field); !ok || got != expectedKey {
		t.Fatalf("TableFields.Get(%v) = %v/%v, want exact Source key %v", field, got, ok, expectedKey)
	}
	if geometry.ExactLenses().Count() != 0 || geometry.DynamicLenses().Count() != 0 ||
		geometry.IndexAccesses().Reads().Count() != 0 || geometry.IndexAccesses().Writes().Count() != 0 {
		t.Fatal("projection fixture unexpectedly retained non-table access geometry rows")
	}
	primitives := view.BinaryPrimitives()
	if !primitives.Available() {
		t.Fatal("valid assembled BinaryPrimitives reported unavailable")
	}
	if primitives.Arithmetic().Count() != 1 {
		t.Fatalf("Arithmetic primitive count = %d, want one retained Binary", primitives.Arithmetic().Count())
	}
	binary := keyspace.MakeTerm(keyspace.FamilyBinary, 1)
	if got, ok := primitives.Arithmetic().At(0); !ok || got != binary {
		t.Fatalf("Arithmetic.At(0) = %v/%v, want %v", got, ok, binary)
	}
	primitive, ok := primitives.Primitive(binary)
	if !ok {
		t.Fatal("retained Binary primitive was not published")
	}
	operation, ok := primitive.Operation()
	if !ok || operation.Op != kind.BinaryAdd || operation.Left != keyspace.MakeTerm(keyspace.FamilyInteger, 1) || operation.Right != keyspace.MakeTerm(keyspace.FamilyInteger, 2) {
		t.Fatalf("Binary primitive operation = %#v/%v, want authored addition", operation, ok)
	}
}

func TestFlowProjectionQueriesFailClosedOnNilResults(t *testing.T) {
	assembly := openProjectionAssembly(t, "projection-nil-access.lua")
	_, flowComponent, _, _, err := assembly.Take()
	if err != nil {
		t.Fatalf("Assembly.Take: %v", err)
	}
	flowComponent.accessGeometry = nil
	if flowComponent.View().AccessGeometry().Available() {
		t.Fatal("nil AccessGeometry reported available")
	}
	if got := flowComponent.View().AccessGeometry().IndexAccesses().Reads().Count(); got != 0 {
		t.Fatalf("nil AccessGeometry Read count = %d, want 0", got)
	}
	if !flowComponent.View().BinaryPrimitives().Available() || flowComponent.View().BinaryPrimitives().Arithmetic().Count() != 1 {
		t.Fatal("nil AccessGeometry also hid the independent nonempty BinaryPrimitives projection")
	}

	assembly = openProjectionAssembly(t, "projection-nil-binary.lua")
	_, flowComponent, _, _, err = assembly.Take()
	if err != nil {
		t.Fatalf("nil BinaryPrimitives Assembly.Take: %v", err)
	}
	flowComponent.binaryPrimitives = nil
	if flowComponent.View().BinaryPrimitives().Available() {
		t.Fatal("nil BinaryPrimitives reported available")
	}
	if !flowComponent.View().AccessGeometry().Available() || flowComponent.View().AccessGeometry().TableFields().Count() != 1 {
		t.Fatal("nil BinaryPrimitives also hid the independent nonempty AccessGeometry projection")
	}
}

func TestFlowProjectionQueriesRejectForeignAccessGeometry(t *testing.T) {
	assemblyA := openProjectionAssembly(t, "projection-splice-access-a.lua")
	sourceA, flowA, _, _, err := assemblyA.Take()
	if err != nil {
		t.Fatalf("Assembly A.Take: %v", err)
	}
	assemblyB := openProjectionAssembly(t, "projection-splice-access-b.lua")
	sourceB, flowB, _, _, err := assemblyB.Take()
	if err != nil {
		t.Fatalf("Assembly B.Take: %v", err)
	}
	if sourceA.View().Identity().ContentID() == sourceB.View().Identity().ContentID() {
		t.Fatal("splice fixtures unexpectedly share Source provenance")
	}
	flowA.accessGeometry = flowB.accessGeometry
	view := flowA.View()
	if view.AccessGeometry().Available() || view.AccessGeometry().TableFields().Count() != 0 {
		t.Fatal("foreign AccessGeometry remained publishable")
	}
	if !view.BinaryPrimitives().Available() || view.BinaryPrimitives().Arithmetic().Count() != 1 {
		t.Fatal("foreign AccessGeometry splice hid the original BinaryPrimitives projection")
	}
}

func TestFlowProjectionQueriesRejectForeignBinaryPrimitives(t *testing.T) {
	assemblyC := openProjectionAssembly(t, "projection-splice-binary-c.lua")
	sourceC, flowC, _, _, err := assemblyC.Take()
	if err != nil {
		t.Fatalf("Assembly C.Take: %v", err)
	}
	assemblyD := openProjectionAssembly(t, "projection-splice-binary-d.lua")
	sourceD, flowD, _, _, err := assemblyD.Take()
	if err != nil {
		t.Fatalf("Assembly D.Take: %v", err)
	}
	if sourceC.View().Identity().ContentID() == sourceD.View().Identity().ContentID() {
		t.Fatal("splice fixtures unexpectedly share Source provenance")
	}
	flowC.binaryPrimitives = flowD.binaryPrimitives
	view := flowC.View()
	if view.BinaryPrimitives().Available() || view.BinaryPrimitives().Arithmetic().Count() != 0 {
		t.Fatal("foreign BinaryPrimitives remained publishable")
	}
	if !view.AccessGeometry().Available() || view.AccessGeometry().TableFields().Count() != 1 {
		t.Fatal("foreign BinaryPrimitives splice hid the original AccessGeometry projection")
	}
}

func openProjectionAssembly(t *testing.T, name string) *Assembly {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	binary := keyspace.MakeTerm(keyspace.FamilyBinary, 1)
	integerOne := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	integerTwo := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	stringField := keyspace.MakeTerm(keyspace.FamilyString, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	valuesForField := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	valuesForReturn := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBinary] = 1
	counts[keyspace.FamilyInteger] = 2
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyKey] = 1
	counts[keyspace.FamilyTable] = 1
	counts[keyspace.FamilyTableField] = 1
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyReturn] = 1

	sourceDraft, err := source.Build(source.Input{
		Name:       name,
		Families:   assemblyFamilySpans(name, counts),
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}, {Kind: keyspace.LiteralInteger, Integer: 2}, {Kind: keyspace.LiteralString, String: "field"}},
		Integer:    []source.IntegerLiteral{{Owner: entry, Value: 1}, {Owner: entry, Value: 2}},
		String:     []source.StringLiteral{{Owner: entry, Value: "field"}},
		Keys:       []source.KeyInput{source.NameKey(entry, "field")},
		Bodies:     []source.BodySource{{Body: entry, Terms: []keyspace.Term{returned}}},
	})
	if err != nil {
		t.Fatalf("projection source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("projection source.Finalizer: %v", err)
	}
	staticComponent, staticView, err := static.Build(static.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("projection static.Build: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("projection imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("projection imports.Finalizer: %v", err)
	}
	flowDraft, err := Build(Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: entry, Fixed: authored.Range{End: 1}}, {Owner: entry, Fixed: authored.Range{Start: 1, End: 3}}},
			Terms: []keyspace.Term{stringField, table, binary},
		},
		Tables: authored.TablesInput{
			Rows:   []authored.Table{{Owner: entry, Fields: authored.Range{End: 1}}},
			Fields: []authored.Field{{Table: table, Key: key, Values: valuesForField, Kind: kind.FieldName}},
			Order:  []keyspace.Term{field},
		},
		Operators: authored.OperatorsInput{Binaries: []authored.Binary{{Owner: entry, Op: kind.BinaryAdd, Left: integerOne, Right: integerTwo}}},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: entry, Values: valuesForReturn}}},
	})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("projection flow.Build: %v", err)
	}
	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, flowDraft, entry)
	if err != nil {
		t.Fatalf("projection Assemble: %v", err)
	}
	return assembly
}

func TestAssemblePreflightFailureAbortsEveryConstructionOwner(t *testing.T) {
	sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft := emptyAssemblyOwners(t, "failed-preflight.lua")
	preimage := sourceFinalizer.Preimage()
	moduleView := moduleFinalizer.View()

	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft, 0)
	if err == nil || assembly != nil {
		t.Fatalf("Assemble(invalid entry) = %#v, %v; want terminal failure", assembly, err)
	}
	if preimage.Identity().ContentID().Available() || !staticView.ContentID().Available() || moduleView.ContentID().Available() {
		t.Fatal("failed Assemble left a construction owner View readable or lost Static view")
	}
	if _, claimErr := draft.claim(); claimErr == nil {
		t.Fatal("failed Assemble left the authored Flow Draft reclaimable")
	}
	if sourceFinalizer.Abort() == nil || moduleFinalizer.Abort() {
		t.Fatal("failed Assemble did not leave every owner finalizer terminal")
	}
}

func TestAssemblyTakeMalformedAndZeroTokensFailClosed(t *testing.T) {
	for name, token := range map[string]*Assembly{
		"nil":       nil,
		"zero":      {},
		"malformed": {state: &assemblyState{source: &source.Component{}}},
	} {
		sourceComponent, flowComponent, staticComponent, moduleComponent, err := token.Take()
		if err == nil || sourceComponent != nil || flowComponent != nil || staticComponent != nil || moduleComponent != nil {
			t.Fatalf("%s Assembly.Take = %p/%p/%p/%p, %v; want all nil and error", name, sourceComponent, flowComponent, staticComponent, moduleComponent, err)
		}
	}
}

func TestAssemblyTakeConcurrentCopiesTransferExactlyOnce(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft := emptyAssemblyOwners(t, "concurrent-take.lua")
	assembly, err := Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, draft, entry)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	type takeResult struct {
		source *source.Component
		flow   *Component
		static *static.Component
		module *imports.Component
		err    error
	}
	const workers = 16
	results := make(chan takeResult, workers)
	for range workers {
		copy := *assembly
		go func(token Assembly) {
			sourceComponent, flowComponent, staticComponent, moduleComponent, takeErr := token.Take()
			results <- takeResult{source: sourceComponent, flow: flowComponent, static: staticComponent, module: moduleComponent, err: takeErr}
		}(copy)
	}
	successes := 0
	for range workers {
		result := <-results
		if result.err == nil {
			successes++
			if result.source == nil || result.flow == nil || result.static == nil || result.module == nil {
				t.Fatal("successful concurrent Take returned a partial quartet")
			}
			continue
		}
		if result.source != nil || result.flow != nil || result.static != nil || result.module != nil {
			t.Fatal("failed concurrent Take returned a partial quartet")
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent Take successes = %d; want exactly one", successes)
	}
}

func TestFlowDraftRejectsDerivedOutcomeInput(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyOutcome] = 1
	if draft, err := Build(Input{Counts: counts}); err == nil || draft != nil {
		t.Fatalf("Build accepted authored Outcome denominator: draft=%#v err=%v", draft, err)
	}
}

func emptyAssemblyOwners(t *testing.T, name string) (source.Finalizer, *static.Component, staticquery.View, imports.Finalizer, *Draft) {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	sourceDraft, err := source.Build(source.Input{
		Name:     name,
		Families: assemblyFamilySpans(name, counts),
		Bodies:   []source.BodySource{{Body: entry}},
	})
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	staticComponent, staticView, err := static.Build(static.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("imports.Finalizer: %v", err)
	}
	flowDraft, err := Build(Input{Counts: counts})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("flow.Build: %v", err)
	}
	return sourceFinalizer, staticComponent, staticView, moduleFinalizer, flowDraft
}

func abortClaimedSiblingOwners(
	t testing.TB,
	sourceFinalizer source.Finalizer,
	moduleFinalizer imports.Finalizer,
) {
	t.Helper()
	if !moduleFinalizer.Abort() {
		t.Error("Module finalizer was not live")
	}
	if err := sourceFinalizer.Abort(); err != nil {
		t.Errorf("Source finalizer was not live: %v", err)
	}
}

func assemblyFamilySpans(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
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
