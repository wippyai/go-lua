package modulecomposition_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	modulecomposition "github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func lawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("schema/module-composition/law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

type lawProgram struct {
	mount, targetMount                          programmount.Program
	request                                     programschema.ModuleRequest
	body                                        programschema.Body
	normal, returned, thrown, yielded, canceled programschema.Outcome
	entry                                       programschema.ModuleEntry
}

func makeLawProgram(t *testing.T) lawProgram {
	t.Helper()
	importID, callID := lawID(t, "import"), lawID(t, "call")
	requestID, valueID := lawID(t, "request"), lawID(t, "value")
	importRow, ok := programschema.NewModuleImport(importID, callID, identity.ContentID{}, 0, 1, false)
	if !ok {
		t.Fatal("module import")
	}
	request, ok := programschema.NewModuleRequest(requestID, importID, valueID, keyspace.Key(7))
	if !ok {
		t.Fatal("module request")
	}
	schemaID := lawID(t, "program-schema")
	catalogID, ok := programcatalog.CatalogID(schemaID)
	if !ok {
		t.Fatal("program catalog")
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("program store")
	}
	bodyID := lawID(t, "entry-body")
	body, ok := programschema.NewBody(bodyID, lawID(t, "body-context"), lawID(t, "body-entry"), identity.ContentID{}, identity.ContentID{}, 0, 1, 0, 0, 0, 5, false)
	if !ok {
		t.Fatal("entry body")
	}
	newOutcome := func(label string, kind programschema.OutcomeKind) programschema.Outcome {
		t.Helper()
		outcome, outcomeOK := programschema.NewOutcome(lawID(t, label), bodyID, identity.ContentID{}, identity.ContentID{}, kind, 0, 0, 0, 0, false, false)
		if !outcomeOK {
			t.Fatalf("%s outcome", label)
		}
		return outcome
	}
	normal := newOutcome("normal-outcome", programschema.OutcomeNormal)
	returned, returnedOK := programschema.NewOutcome(lawID(t, "return-outcome"), bodyID, identity.ContentID{}, identity.ContentID{}, programschema.OutcomeReturn, 0, 0, 0, 1, false, false)
	if !returnedOK {
		t.Fatal("return outcome")
	}
	returnPoint, returnPointOK := programschema.NewOutcomePoint(returned.ID(), lawID(t, "return-outcome-point"))
	if !returnPointOK {
		t.Fatal("return outcome point")
	}
	thrown := newOutcome("throw-outcome", programschema.OutcomeThrow)
	yielded := newOutcome("yield-outcome", programschema.OutcomeYield)
	canceled := newOutcome("cancel-outcome", programschema.OutcomeCancel)
	entry, ok := programschema.NewModuleEntry(lawID(t, "return-entry"), returned.ID(), 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("return entry")
	}
	dispatchPointID := lawID(t, "call-dispatch-point")
	summaryPointID := lawID(t, "call-summary-point")
	returnPointID := lawID(t, "call-effect-point")
	basePointID := lawID(t, "call-base-point")
	callOccurrence, ok := programschema.NewOccurrence(programschema.OccurrenceCall, callID, bodyID, 0, 0, 1, 0, 0, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !ok {
		t.Fatal("call occurrence")
	}
	dispatchPoint, ok := programschema.NewOccurrencePoint(dispatchPointID)
	if !ok {
		t.Fatal("call dispatch occurrence point")
	}
	basePoint, basePointOK := programschema.NewOccurrencePoint(basePointID)
	summaryPoint, summaryPointOK := programschema.NewOccurrencePoint(summaryPointID)
	effectPoint, effectPointOK := programschema.NewOccurrencePoint(returnPointID)
	dispatchRule, ok := programschema.NewRuleOccurrenceWithInputs(schema.Key("module-call-transition-law"), schema.Key("module-call-transition-law-axis"), 0, dispatchPointID, []identity.ContentID{basePointID}, programissuance.StageCallDispatch, programissuance.InputPreviousStage, identity.ContentID{}, true, programschema.RuleOccurrenceSource{})
	summaryRule, summaryRuleOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("module-call-transition-summary-law"), schema.Key("module-call-transition-law-axis"), 0, summaryPointID, []identity.ContentID{dispatchPointID}, programissuance.StageCallSummary, programissuance.InputCallDispatchStage, identity.ContentID{}, true, programschema.RuleOccurrenceSource{})
	returnRule, returnRuleOK := programschema.NewRuleOccurrenceWithInputs(schema.Key("module-call-transition-effect-law"), schema.Key("module-call-transition-law-axis"), 0, returnPointID, []identity.ContentID{summaryPointID}, programissuance.StageCallEffect, programissuance.InputCallSummaryStage, identity.ContentID{}, true, programschema.RuleOccurrenceSource{})
	if !ok || !basePointOK || !summaryPointOK || !effectPointOK || !summaryRuleOK || !returnRuleOK {
		t.Fatal("call dispatch rule occurrence")
	}
	frozen, ok := (programpublication.Publication{
		ModuleImports:    []programschema.ModuleImport{importRow},
		ModuleRequests:   []programschema.ModuleRequest{request},
		Occurrences:      []programschema.Occurrence{callOccurrence},
		OccurrencePoints: []programschema.OccurrencePoint{basePoint, dispatchPoint, summaryPoint, effectPoint},
		RuleOccurrences:  []programschema.RuleOccurrence{dispatchRule, summaryRule, returnRule},
		Bodies:           []programschema.Body{body},
		Outcomes:         []programschema.Outcome{normal, returned, thrown, yielded, canceled},
		OutcomePoints:    []programschema.OutcomePoint{returnPoint},
		ModuleEntries:    []programschema.ModuleEntry{entry},
	}).Seal(catalogID, store)
	if !ok {
		t.Fatal("program publication")
	}
	program := programschema.Program{
		Frozen: frozen, ArtifactID: lawID(t, "artifact"), ProgramID: lawID(t, "program"), SchemaID: schemaID,
		EntryBodyID: body.ID(),
	}
	mount := programmount.Program{ModuleKey: lawID(t, "source-module"), Program: program}
	if !mount.Available() {
		t.Fatal("mount")
	}
	targetMount := mount
	targetMount.ModuleKey = lawID(t, "target-module")
	return lawProgram{mount: mount, targetMount: targetMount, request: request, body: body, normal: normal, returned: returned, thrown: thrown, yielded: yielded, canceled: canceled, entry: entry}
}

func makeGeneration(t *testing.T) (lawProgram, modulecomposition.ResolvedImport, modulecomposition.CacheIngress, modulecomposition.InitGeneration) {
	t.Helper()
	program := makeLawProgram(t)
	linkID := lawID(t, "link")
	targetKey := program.targetMount.ModuleKey
	resolved, ok := modulecomposition.NewResolvedImport(linkID, program.mount, program.request, targetKey)
	if !ok {
		t.Fatal("resolved import")
	}
	actorID, representativeID := lawID(t, "actor"), lawID(t, "representative-instance")
	fromContext, fromContextOK := executioncontext.NewContext(linkID, program.mount.ModuleKey, actorID, representativeID)
	toContext, toContextOK := executioncontext.NewContext(linkID, program.targetMount.ModuleKey, actorID, representativeID)
	if !fromContextOK || !toContextOK {
		t.Fatal("execution contexts")
	}
	cache, ok := modulecomposition.NewCacheIngress(resolved, lawID(t, "from-root"), lawID(t, "to-root"), fromContext, toContext)
	if !ok {
		t.Fatal("cache ingress")
	}
	generation, ok := modulecomposition.NewInitGeneration(cache, program.targetMount, program.body)
	if !ok {
		t.Fatal("init generation")
	}
	return program, resolved, cache, generation
}

func makeRows(t *testing.T) (modulecomposition.ResolvedImport, modulecomposition.CacheIngress, modulecomposition.InitGeneration, modulecomposition.InitOutcome, modulecomposition.InitTerminal) {
	t.Helper()
	program, resolved, cache, generation := makeGeneration(t)
	outcome, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, program.thrown)
	if !ok {
		t.Fatal("init outcome")
	}
	terminal, ok := modulecomposition.NewInitTerminal(outcome)
	if !ok {
		t.Fatal("init terminal")
	}
	return resolved, cache, generation, outcome, terminal
}

func TestResolvedRowsAreCanonicalAndLinkQualified(t *testing.T) {
	resolved, cache, generation, outcome, terminal := makeRows(t)
	for name, available := range map[string]bool{
		"resolved import": resolved.Available(), "cache ingress": cache.Available(), "generation": generation.Available(),
		"outcome": outcome.Available(), "terminal": terminal.Available(),
	} {
		if !available {
			t.Fatalf("%s unavailable", name)
		}
	}
	if resolved.LinkID() != cache.LinkID() || cache.LinkID() != generation.LinkID() || generation.LinkID() != outcome.LinkID() || outcome.LinkID() != terminal.LinkID() {
		t.Fatal("row family lost its Link identity")
	}
	if cache.ImportID() != resolved.ID() || cache.RequestID() != resolved.RequestID() {
		t.Fatal("cache ingress lost the exact import/request join")
	}
	if cache.RepresentativeInstanceID() != lawID(t, "representative-instance") {
		t.Fatal("cache ingress lost the source representative identity")
	}
	otherRepresentative := lawID(t, "other-representative")
	otherFromContext, fromContextOK := executioncontext.NewContext(cache.LinkID(), cache.SourceModuleKey(), cache.ActorID(), otherRepresentative)
	otherToContext, toContextOK := executioncontext.NewContext(cache.LinkID(), cache.TargetModuleKey(), cache.ActorID(), otherRepresentative)
	if !fromContextOK || !toContextOK {
		t.Fatal("other execution contexts")
	}
	otherCache, ok := modulecomposition.NewCacheIngress(resolved, cache.FromRootID(), cache.ToRootID(), otherFromContext, otherToContext)
	if !ok || otherCache.ID() == cache.ID() {
		t.Fatal("cache ingress identity ignored its representative")
	}
	if generation.CacheIngressID() != cache.ID() || generation.ModuleKey() != resolved.TargetModuleKey() {
		t.Fatal("generation lost the cache target join")
	}
	if outcome.GenerationID() != generation.ID() || terminal.GenerationID() != generation.ID() || terminal.OutcomeID() != outcome.OutcomeID() {
		t.Fatal("outcome/terminal join is not exact")
	}
	if outcome.Kind() != programschema.OutcomeThrow {
		t.Fatal("canonical throw outcome kind was not retained")
	}

	otherProgram := makeLawProgram(t)
	otherLink := lawID(t, "other-link")
	other, ok := modulecomposition.NewResolvedImport(otherLink, otherProgram.mount, otherProgram.request, resolved.TargetModuleKey())
	if !ok {
		t.Fatal("other-link row")
	}
	if other.ID() == resolved.ID() {
		t.Fatal("two Links share one resolved import identity")
	}
}

func TestResolvedImportRejectsARequestFromAnotherProgram(t *testing.T) {
	program := makeLawProgram(t)
	foreign := program.request
	foreignImport, ok := programschema.NewModuleImport(lawID(t, "foreign-import"), lawID(t, "foreign-call"), identity.ContentID{}, 0, 1, false)
	if !ok {
		t.Fatal("foreign import")
	}
	foreign, ok = programschema.NewModuleRequest(lawID(t, "foreign-request"), foreignImport.ID(), lawID(t, "foreign-value"), keyspace.Key(8))
	if !ok {
		t.Fatal("foreign request")
	}
	if _, ok := modulecomposition.NewResolvedImport(lawID(t, "link"), program.mount, foreign, lawID(t, "target")); ok {
		t.Fatal("request from another Program joined the mount")
	}
}

func TestCacheIngressRejectsForeignOrMisplacedTargetContext(t *testing.T) {
	program := makeLawProgram(t)
	linkID := lawID(t, "link")
	resolved, ok := modulecomposition.NewResolvedImport(linkID, program.mount, program.request, program.targetMount.ModuleKey)
	if !ok {
		t.Fatal("resolved import")
	}
	actorID, representativeID := lawID(t, "actor"), lawID(t, "representative-instance")
	fromContext, fromOK := executioncontext.NewContext(linkID, program.mount.ModuleKey, actorID, representativeID)
	foreignTarget, foreignOK := executioncontext.NewContext(lawID(t, "foreign-link"), program.targetMount.ModuleKey, actorID, representativeID)
	if !fromOK || !foreignOK {
		t.Fatal("contexts")
	}
	if _, ok := modulecomposition.NewCacheIngress(resolved, lawID(t, "from-root"), lawID(t, "to-root"), fromContext, foreignTarget); ok {
		t.Fatal("foreign target context crossed the Link fence")
	}
	misplacedTarget, misplacedOK := executioncontext.NewContext(linkID, program.mount.ModuleKey, actorID, lawID(t, "other-representative"))
	if !misplacedOK {
		t.Fatal("misplaced target")
	}
	if _, ok := modulecomposition.NewCacheIngress(resolved, lawID(t, "from-root-2"), lawID(t, "to-root-2"), fromContext, misplacedTarget); ok {
		t.Fatal("target context for source module crossed the endpoint fence")
	}
	targetRepresentative := lawID(t, "target-representative")
	exactTarget, exactOK := executioncontext.NewContext(linkID, program.targetMount.ModuleKey, actorID, targetRepresentative)
	if !exactOK {
		t.Fatal("exact target context")
	}
	cache, cacheOK := modulecomposition.NewCacheIngress(resolved, lawID(t, "from-root-3"), lawID(t, "to-root-3"), fromContext, exactTarget)
	if !cacheOK || cache.ToContextID() != exactTarget.ID() {
		t.Fatal("target representative was reconstructed from source context")
	}
}

func TestGenerationAuthenticatesTheCanonicalEntryBody(t *testing.T) {
	program, _, cache, _ := makeGeneration(t)
	callable, ok := programschema.NewBody(lawID(t, "callable-body"), lawID(t, "callable-context"), lawID(t, "callable-entry"), lawID(t, "callable-function"), lawID(t, "callable-formal"), 0, 1, 0, 0, 0, 1, true)
	if !ok {
		t.Fatal("callable body")
	}
	if _, ok := modulecomposition.NewInitGeneration(cache, program.targetMount, callable); ok {
		t.Fatal("callable body admitted as module entry body")
	}
	forged, ok := programschema.NewBody(program.body.ID(), lawID(t, "forged-context"), program.body.EntryID(), identity.ContentID{}, identity.ContentID{}, 0, 1, 0, 0, 0, 5, false)
	if !ok {
		t.Fatal("forged body")
	}
	if _, ok := modulecomposition.NewInitGeneration(cache, program.targetMount, forged); ok {
		t.Fatal("forged body with canonical ID admitted")
	}
}

func TestOutcomeAuthenticatesCanonicalBodyOutcomeOrReturnEntry(t *testing.T) {
	program, _, _, generation := makeGeneration(t)
	normal, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, program.normal)
	if !ok || normal.Kind() != programschema.OutcomeNormal {
		t.Fatal("canonical normal outcome was rejected")
	}
	returnOutcome, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, program.returned)
	if !ok {
		t.Fatal("canonical return outcome was rejected")
	}
	if ordinal, ok := returnOutcome.ReturnOrdinal(); !ok || ordinal != 1 {
		t.Fatalf("canonical return ordinal = %d/%v, want 1/true", ordinal, ok)
	}
	fromEntry, ok := modulecomposition.NewInitOutcomeFromModuleEntry(generation, program.targetMount, program.entry)
	if !ok || fromEntry.ID() != returnOutcome.ID() {
		t.Fatal("canonical ModuleEntry did not resolve the same return outcome")
	}
	foreign, ok := programschema.NewOutcome(lawID(t, "foreign-outcome"), lawID(t, "foreign-body"), identity.ContentID{}, identity.ContentID{}, programschema.OutcomeNormal, 0, 0, 0, 0, false, false)
	if !ok {
		t.Fatal("foreign outcome")
	}
	if _, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, foreign); ok {
		t.Fatal("foreign body outcome admitted")
	}
	forged, ok := programschema.NewOutcome(program.thrown.ID(), program.body.ID(), identity.ContentID{}, identity.ContentID{}, programschema.OutcomeYield, 0, 0, 0, 0, false, false)
	if !ok {
		t.Fatal("forged outcome")
	}
	if _, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, forged); ok {
		t.Fatal("forged canonical outcome identity admitted")
	}
	wrongEntry, ok := programschema.NewModuleEntry(program.entry.ID(), program.normal.ID(), 1, 0, 0, 0, 0, 0, 0, 0)
	if !ok {
		t.Fatal("wrong return entry")
	}
	if _, ok := modulecomposition.NewInitOutcomeFromModuleEntry(generation, program.targetMount, wrongEntry); ok {
		t.Fatal("ModuleEntry with non-return ReturnID admitted")
	}
}

func TestTerminalAdmitsOnlyCanonicalThrowAndCancel(t *testing.T) {
	program, _, _, generation := makeGeneration(t)
	for _, kind := range []programschema.OutcomeKind{programschema.OutcomeNormal, programschema.OutcomeYield, programschema.OutcomeReturn} {
		var source programschema.Outcome
		switch kind {
		case programschema.OutcomeNormal:
			source = program.normal
		case programschema.OutcomeYield:
			source = program.yielded
		case programschema.OutcomeReturn:
			source = program.returned
		}
		var row modulecomposition.InitOutcome
		var ok bool
		if kind == programschema.OutcomeReturn {
			row, ok = modulecomposition.NewInitOutcomeFromModuleEntry(generation, program.targetMount, program.entry)
		} else {
			row, ok = modulecomposition.NewInitOutcome(generation, program.targetMount, source)
		}
		if !ok {
			t.Fatalf("build %v outcome", kind)
		}
		if _, ok := modulecomposition.NewInitTerminal(row); ok {
			t.Fatalf("%v admitted as terminal", kind)
		}
	}
	cancel, ok := modulecomposition.NewInitOutcome(generation, program.targetMount, program.canceled)
	if !ok {
		t.Fatal("canonical cancel outcome")
	}
	if _, ok := modulecomposition.NewInitTerminal(cancel); !ok {
		t.Fatal("canonical cancel rejected as terminal")
	}
}

func TestContentCanonicalizesMembersAndRejectsDuplicateRows(t *testing.T) {
	resolved, cache, generation, outcome, terminal := makeRows(t)
	linkID := resolved.LinkID()
	denominator, ok := modulecomposition.ImportDenominatorID(linkID)
	if !ok {
		t.Fatal("import denominator")
	}
	content, ok := modulecomposition.ImportContent([]modulecomposition.ResolvedImport{resolved}, denominator)
	if !ok || len(content.Rows) != 1 || len(content.Members) != 1 || content.Members[0] != resolved.ID() {
		t.Fatal("import content did not publish its one stable key")
	}
	if _, ok := modulecomposition.ImportContent([]modulecomposition.ResolvedImport{resolved, resolved}, denominator); ok {
		t.Fatal("duplicate import row accepted")
	}
	if _, ok := modulecomposition.CacheContent([]modulecomposition.CacheIngress{cache, cache}, mustDenominator(t, modulecomposition.CacheDenominatorID, linkID)); ok {
		t.Fatal("duplicate cache row accepted")
	}
	if _, ok := modulecomposition.GenerationContent([]modulecomposition.InitGeneration{generation, generation}, mustDenominator(t, modulecomposition.GenerationDenominatorID, linkID)); ok {
		t.Fatal("duplicate generation row accepted")
	}
	if _, ok := modulecomposition.OutcomeContent([]modulecomposition.InitOutcome{outcome, outcome}, mustDenominator(t, modulecomposition.OutcomeDenominatorID, linkID)); ok {
		t.Fatal("duplicate outcome row accepted")
	}
	if _, ok := modulecomposition.TerminalContent([]modulecomposition.InitTerminal{terminal, terminal}, mustDenominator(t, modulecomposition.TerminalDenominatorID, linkID)); ok {
		t.Fatal("duplicate terminal row accepted")
	}
}

// An empty Link composition is still a publication: its denominator names the
// empty sparse universe, so consumers can distinguish a sealed empty column
// from a column that was never published. A key outside that empty universe
// remains a sparse miss rather than being laundered into a hit or absence.
func TestEmptyImportPublicationIsAValidClosedWorldColumn(t *testing.T) {
	linkID := lawID(t, "empty-link")
	denominator := mustDenominator(t, modulecomposition.ImportDenominatorID, linkID)
	content, ok := modulecomposition.ImportContent(nil, denominator)
	if !ok || content.Rows == nil || len(content.Rows) != 0 || len(content.Members) != 0 || content.Denominator != denominator {
		t.Fatal("empty import composition did not publish its closed-world denominator")
	}
	schemaID := lawID(t, "empty-schema")
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	builder := snapshot.NewBuilder(schemaID, store, identity.Generation(1))
	address := modulecomposition.ImportAxis(schemaID, 0)
	if err := snapshot.PutColumn(&builder, address, content); err != nil {
		t.Fatalf("put empty import column: %v", err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal empty import column: %v", err)
	}
	if _, status := snapshot.Read(&published, address, lawID(t, "outside-empty-universe")); status != snapshot.ReadMiss {
		t.Fatalf("empty sparse universe returned %s instead of a miss", status)
	}
}

func mustDenominator(t *testing.T, derive func(identity.ContentID) (identity.ContentID, bool), linkID identity.ContentID) identity.ContentID {
	t.Helper()
	id, ok := derive(linkID)
	if !ok {
		t.Fatal("denominator")
	}
	return id
}

func TestAxesAreFrozenLinkLifetimeSparseAndSelfWritten(t *testing.T) {
	checks := []struct {
		name string
		spec axis.Spec[any]
	}{
		{"import", asAny(modulecomposition.ImportAxisEntry[modulecomposition.ResolvedImport]())},
		{"cache", asAny(modulecomposition.CacheAxisEntry[modulecomposition.CacheIngress]())},
		{"module-call-transition", asAny(modulecomposition.ModuleCallTransitionAxisEntry[modulecomposition.ModuleCallTransition]())},
		{"generation", asAny(modulecomposition.GenerationAxisEntry[modulecomposition.InitGeneration]())},
		{"outcome", asAny(modulecomposition.OutcomeAxisEntry[modulecomposition.InitOutcome]())},
		{"terminal", asAny(modulecomposition.TerminalAxisEntry[modulecomposition.InitTerminal]())},
	}
	seenOutputs := make(map[string]bool)
	for _, check := range checks {
		if check.spec.Storage != axis.StorageEngine || check.spec.Cardinality != axis.CardinalitySparse || check.spec.Lifetime != axis.LifetimeLink ||
			check.spec.Mutability != axis.MutabilityFrozen || check.spec.Concurrency != axis.ConcurrencyShared || len(check.spec.Frame.Outputs) != 1 {
			t.Fatalf("%s declaration does not state frozen Link-lifetime sparse storage", check.name)
		}
		output := check.spec.Frame.Outputs[0]
		if output.Writer != check.spec.Key || seenOutputs[string(output.Key)] {
			t.Fatalf("%s output is not uniquely self-written", check.name)
		}
		seenOutputs[string(output.Key)] = true
	}
	if got := asAny(modulecomposition.ImportAxisEntry[modulecomposition.ResolvedImport]()).Dependencies; len(got) != 1 || got[0] != programmount.AxisKey {
		t.Fatal("import axis dependency is not programmount")
	}
	if got := asAny(modulecomposition.CacheAxisEntry[modulecomposition.CacheIngress]()).Dependencies; len(got) != 1 || got[0] != modulecomposition.ImportAxisKey {
		t.Fatal("cache axis dependency is not resolved import")
	}
	if got := asAny(modulecomposition.ModuleCallTransitionAxisEntry[modulecomposition.ModuleCallTransition]()).Dependencies; len(got) != 2 || got[0] != modulecomposition.CacheAxisKey || got[1] != programmount.AxisKey {
		t.Fatal("module-call transition axis dependency chain is incomplete")
	}
	if got := asAny(modulecomposition.GenerationAxisEntry[modulecomposition.InitGeneration]()).Dependencies; len(got) != 2 || got[0] != modulecomposition.CacheAxisKey || got[1] != programmount.AxisKey {
		t.Fatal("generation axis dependency chain is incomplete")
	}
	if got := asAny(modulecomposition.OutcomeAxisEntry[modulecomposition.InitOutcome]()).Dependencies; len(got) != 1 || got[0] != modulecomposition.GenerationAxisKey {
		t.Fatal("outcome axis dependency is not generation")
	}
	if got := asAny(modulecomposition.TerminalAxisEntry[modulecomposition.InitTerminal]()).Dependencies; len(got) != 1 || got[0] != modulecomposition.OutcomeAxisKey {
		t.Fatal("terminal axis dependency is not outcome")
	}
}

func asAny[A any](spec axis.Spec[A]) axis.Spec[any] {
	return axis.Spec[any]{Key: spec.Key, Storage: spec.Storage, Cardinality: spec.Cardinality, Lifetime: spec.Lifetime, Mutability: spec.Mutability, Concurrency: spec.Concurrency, Dependencies: spec.Dependencies, Frame: spec.Frame, Semantic: spec.Semantic}
}

func TestAllRowsSealAndReadThroughTheirOwnSnapshotColumns(t *testing.T) {
	resolved, cache, generation, outcome, terminal := makeRows(t)
	linkID := resolved.LinkID()
	denominators := []identity.ContentID{
		mustDenominator(t, modulecomposition.ImportDenominatorID, linkID),
		mustDenominator(t, modulecomposition.CacheDenominatorID, linkID),
		mustDenominator(t, modulecomposition.GenerationDenominatorID, linkID),
		mustDenominator(t, modulecomposition.OutcomeDenominatorID, linkID),
		mustDenominator(t, modulecomposition.TerminalDenominatorID, linkID),
	}
	schemaID := lawID(t, "composition-schema")
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	builder := snapshot.NewBuilder(schemaID, store, identity.Generation(1))
	importContent, importOK := modulecomposition.ImportContent([]modulecomposition.ResolvedImport{resolved}, denominators[0])
	if !importOK {
		t.Fatal("import content")
	}
	if err := snapshot.PutColumn(&builder, modulecomposition.ImportAxis(schemaID, 0), importContent); err != nil {
		t.Fatal(err)
	}
	cacheContent, cacheOK := modulecomposition.CacheContent([]modulecomposition.CacheIngress{cache}, denominators[1])
	if !cacheOK {
		t.Fatal("cache content")
	}
	if err := snapshot.PutColumn(&builder, modulecomposition.CacheAxis(schemaID, 1), cacheContent); err != nil {
		t.Fatal(err)
	}
	generationContent, generationOK := modulecomposition.GenerationContent([]modulecomposition.InitGeneration{generation}, denominators[2])
	if !generationOK {
		t.Fatal("generation content")
	}
	if err := snapshot.PutColumn(&builder, modulecomposition.GenerationAxis(schemaID, 2), generationContent); err != nil {
		t.Fatal(err)
	}
	outcomeContent, outcomeOK := modulecomposition.OutcomeContent([]modulecomposition.InitOutcome{outcome}, denominators[3])
	if !outcomeOK {
		t.Fatal("outcome content")
	}
	if err := snapshot.PutColumn(&builder, modulecomposition.OutcomeAxis(schemaID, 3), outcomeContent); err != nil {
		t.Fatal(err)
	}
	terminalContent, terminalOK := modulecomposition.TerminalContent([]modulecomposition.InitTerminal{terminal}, denominators[4])
	if !terminalOK {
		t.Fatal("terminal content")
	}
	if err := snapshot.PutColumn(&builder, modulecomposition.TerminalAxis(schemaID, 4), terminalContent); err != nil {
		t.Fatal(err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := modulecomposition.ImportAt(&published, modulecomposition.ImportAxis(schemaID, 0), resolved.ID()); !ok || got.ID() != resolved.ID() {
		t.Fatal("resolved import did not read back")
	}
	if got, ok := modulecomposition.CacheAt(&published, modulecomposition.CacheAxis(schemaID, 1), cache.ID()); !ok || got.ID() != cache.ID() {
		t.Fatal("cache ingress did not read back")
	}
	if got, ok := modulecomposition.GenerationAt(&published, modulecomposition.GenerationAxis(schemaID, 2), generation.ID()); !ok || got.ID() != generation.ID() {
		t.Fatal("generation did not read back")
	}
	if got, ok := modulecomposition.OutcomeAt(&published, modulecomposition.OutcomeAxis(schemaID, 3), outcome.ID()); !ok || got.ID() != outcome.ID() {
		t.Fatal("outcome did not read back")
	}
	if got, ok := modulecomposition.TerminalAt(&published, modulecomposition.TerminalAxis(schemaID, 4), terminal.ID()); !ok || got.ID() != terminal.ID() {
		t.Fatal("terminal did not read back")
	}
	if status := func() snapshot.ReadStatus {
		_, status := snapshot.Read(&published, modulecomposition.ImportAxis(schemaID, 0), lawID(t, "outside"))
		return status
	}(); status != snapshot.ReadMiss {
		t.Fatalf("sparse row column incorrectly proved absence: %s", status)
	}
}
