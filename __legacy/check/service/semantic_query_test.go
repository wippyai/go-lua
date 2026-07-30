package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	lifecyclefx "github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/embedding"
	enginestate "github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const semanticQuerySource = `local captured: number = 1
local function outer(value: number): number
	local localValue: number = value
	local function inner(): number
		captured = captured + localValue
		return value
	end
	return inner()
end
local result = outer(2)
return { outer = outer, result = result }
`

func solveSemanticQueryFixture(t *testing.T, source string) (*BatchSession, ResultTag) {
	t.Helper()
	session := NewBatchSession()
	return solveSemanticQueryFixtureWithSession(t, session, source)
}

func solveSemanticQueryFixtureWithSession(t *testing.T, session *BatchSession, source string) (*BatchSession, ResultTag) {
	t.Helper()
	input := UnitInput{
		ID:         "semantic-query",
		ModulePath: "example/semantic-query",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{
			"main.lua": []byte(source),
		},
	}
	if _, err := session.UpsertUnit(context.Background(), input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	tag, err := session.EnsureSolved(context.Background(), SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	if err != nil {
		t.Fatalf("EnsureSolved: %v", err)
	}
	return session, tag
}

func TestSemanticProjectionCanBeDisabledWithoutChangingCoreResult(t *testing.T) {
	fullSession, fullTag := solveSemanticQueryFixtureWithSession(t, NewBatchSession(), semanticQuerySource)
	coreSession, coreTag := solveSemanticQueryFixtureWithSession(t, NewBatchSession(WithoutSemanticProjection()), semanticQuerySource)

	full, ok := fullSession.LastComplete(context.Background(), ResultRequest{Selector: selectorFor(fullTag)})
	if !ok {
		t.Fatal("default session has no completed result")
	}
	core, ok := coreSession.LastComplete(context.Background(), ResultRequest{Selector: selectorFor(coreTag)})
	if !ok {
		t.Fatal("projection-disabled session has no completed result")
	}
	if full.snapshot.semantic == nil || len(full.snapshot.semantic.binders) == 0 {
		t.Fatal("default session did not publish semantic projection")
	}
	if core.snapshot.semantic != nil {
		t.Fatalf("projection-disabled semantic snapshot = %#v, want nil", core.snapshot.semantic)
	}

	fullPath, fullDigest, fullManifest := full.ManifestBytes()
	corePath, coreDigest, coreManifest := core.ManifestBytes()
	if fullPath != corePath || fullDigest != coreDigest || !reflect.DeepEqual(fullManifest, coreManifest) {
		t.Fatalf("core manifest changed when semantic projection was disabled: default=%q/%s/%d disabled=%q/%s/%d", fullPath, fullDigest, len(fullManifest), corePath, coreDigest, len(coreManifest))
	}
	if !reflect.DeepEqual(full.RenderedDiagnostics(), core.RenderedDiagnostics()) {
		t.Fatalf("diagnostics changed when semantic projection was disabled\ndefault=%#v\ndisabled=%#v", full.RenderedDiagnostics(), core.RenderedDiagnostics())
	}
	if !reflect.DeepEqual(full.Judgments(), core.Judgments()) {
		t.Fatalf("judgments changed when semantic projection was disabled\ndefault=%#v\ndisabled=%#v", full.Judgments(), core.Judgments())
	}
	if !reflect.DeepEqual(full.Tag(), core.Tag()) {
		t.Fatalf("result tag changed when semantic projection was disabled\ndefault=%#v\ndisabled=%#v", full.Tag(), core.Tag())
	}

	tokens, err := coreSession.SemanticTokens(context.Background(), SemanticTokensRequest{Selector: selectorFor(coreTag)})
	if err != nil {
		t.Fatalf("SemanticTokens: %v", err)
	}
	if len(tokens.Tokens) != 0 || tokens.Meta.Tag.SolveSeq != coreTag.SolveSeq {
		t.Fatalf("projection-disabled semantic tokens = %#v, want empty response with selected result meta", tokens)
	}
	lookup, err := coreSession.PositionLookup(context.Background(), PositionLookupRequest{Selector: selectorFor(coreTag)})
	if err != nil {
		t.Fatalf("PositionLookup: %v", err)
	}
	if lookup.Found || lookup.Meta.Tag.SolveSeq != coreTag.SolveSeq {
		t.Fatalf("projection-disabled position lookup = %#v, want not-found response with selected result meta", lookup)
	}
}

func TestSemanticQueryBinderOccurrencesIncludeCaptures(t *testing.T) {
	session, tag := solveSemanticQueryFixture(t, semanticQuerySource)
	result, err := session.BinderOccurrences(context.Background(), BinderOccurrencesRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("BinderOccurrences: %v", err)
	}
	captured := binderNamed(result.Binders, "captured")
	if captured == nil || !captured.Definition.Valid() || captured.Kind != BinderLocal {
		t.Fatalf("captured binder = %#v, want local with definition", captured)
	}
	if !captured.ModuleLocal || !captured.Scope.Valid() {
		t.Fatalf("captured binder scope = %#v, want module-local binder with lexical scope", captured)
	}
	if got := countOccurrenceRole(captured.Occurrences, BinderCapture); got != 2 {
		t.Fatalf("captured capture occurrences = %d, want both read/write capture spans: %#v", got, captured.Occurrences)
	}
	value := binderNamed(result.Binders, "value")
	if value == nil || value.Kind != BinderParam || !value.Definition.Valid() {
		t.Fatalf("value binder = %#v, want parameter definition", value)
	}
	if !value.Scope.Valid() || len(value.Occurrences) == 0 || !value.Occurrences[0].Scope.Valid() {
		t.Fatalf("value binder scope facts = %#v, want definition and occurrence scopes", value)
	}
	if got := countOccurrenceRole(value.Occurrences, BinderCapture); got != 1 {
		t.Fatalf("value capture occurrences = %d, want 1: %#v", got, value.Occurrences)
	}
}

func TestSemanticQueryBinderProjectionUsesOwnedDeclarationsAndCertification(t *testing.T) {
	source := `local function f(first, second)
	return second + first
end
local normal = second
return f(normal, 2)
`
	session, tag := solveSemanticQueryFixture(t, source)
	result, err := session.BinderOccurrences(context.Background(), BinderOccurrencesRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("BinderOccurrences: %v", err)
	}

	second := binderNamed(result.Binders, "second")
	if second == nil {
		t.Fatal("second parameter binder missing")
	}
	headerOffset := strings.Index(source, "second)")
	if headerOffset < 0 {
		t.Fatal("second parameter header missing")
	}
	if second.Definition.Document != embedding.FileDocument("main.lua") || second.Definition.ContentDigest != embedding.DigestBytes([]byte(source)) || second.Definition.ByteSpan != (embedding.ByteSpan{StartByte: headerOffset, EndByte: headerOffset + len("second")}) {
		t.Fatalf("second definition = %#v, want exact header declaration at byte %d", second.Definition, headerOffset)
	}
	if !second.OccurrencesComplete || !second.Renameable {
		t.Fatalf("second certification = complete:%v renameable:%v, want both true", second.OccurrencesComplete, second.Renameable)
	}
	for _, occurrence := range second.Occurrences {
		if occurrence.Location.ByteSpan == second.Definition.ByteSpan {
			t.Fatalf("second occurrence duplicates declaration: %#v", second)
		}
	}

	normal := binderNamed(result.Binders, "normal")
	if normal == nil || !normal.OccurrencesComplete || !normal.Renameable {
		t.Fatalf("normal local certification = %#v, want a complete renameable local", normal)
	}
}

func TestSemanticQuerySyntheticBindersAreNeverRenameable(t *testing.T) {
	tests := []struct {
		name   string
		source string
		binder string
	}{
		{
			name: "implicit self",
			source: `local t = {}
function t:m(v)
	return self, v
end
return t
`,
			binder: "self",
		},
		{
			name: "vararg",
			source: `local function f(...)
	return ...
end
return f(1, 2)
`,
			binder: "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, tag := solveSemanticQueryFixture(t, tt.source)
			result, err := session.BinderOccurrences(context.Background(), BinderOccurrencesRequest{Selector: selectorFor(tag)})
			if err != nil {
				t.Fatalf("BinderOccurrences: %v", err)
			}
			binder := binderNamed(result.Binders, tt.binder)
			if binder == nil {
				t.Fatalf("%s binder missing", tt.binder)
			}
			if binder.Renameable {
				t.Fatalf("synthetic binder = %#v, must not be renameable", binder)
			}
			if tt.binder == "self" && binder.Definition.Valid() {
				t.Fatalf("implicit self definition = %#v, must not invent a source declaration", binder.Definition)
			}
			if !binder.OccurrencesComplete || len(binder.Occurrences) == 0 {
				t.Fatalf("synthetic binder occurrences = %#v, want complete exact occurrences", binder)
			}
		})
	}
}

func TestSemanticQueryLocationIdentityDisambiguatesSameLabelDocuments(t *testing.T) {
	entryDocument := embedding.MemDocument("entry")
	otherDocument := embedding.MemDocument("other")
	entrySource := []byte("local target = 1\nreturn target\n")
	otherSource := []byte("return 0\n")
	for attempt := 0; attempt < 100; attempt++ {
		session := NewBatchSession()
		input := UnitInput{
			ID:            UnitID("same-label"),
			ModulePath:    "example/same-label",
			EntryDocument: entryDocument,
			Sources: map[embedding.DocumentID]embedding.SourceSnapshot{
				entryDocument: {Document: entryDocument, Content: entrySource},
				otherDocument: {Document: otherDocument, Content: otherSource},
			},
			DocumentLabels: embedding.StaticLabels{
				entryDocument: "same.lua",
				otherDocument: "same.lua",
			},
		}
		if _, err := session.UpsertUnit(context.Background(), input); err != nil {
			t.Fatalf("UpsertUnit: %v", err)
		}
		tag, err := session.EnsureSolved(context.Background(), SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
		if err != nil {
			t.Fatalf("EnsureSolved: %v", err)
		}
		offset := strings.LastIndex(string(entrySource), "target")
		lookup, err := session.PositionLookup(context.Background(), PositionLookupRequest{
			Selector:      selectorFor(tag),
			Document:      entryDocument,
			ContentDigest: embedding.DigestBytes(entrySource),
			Position:      SourcePosition{Offset: offset},
		})
		if err != nil {
			t.Fatalf("PositionLookup: %v", err)
		}
		if !lookup.Found || lookup.Binder == nil || lookup.Binder.Name != "target" {
			t.Fatalf("attempt %d PositionLookup = %#v, want entry target binder", attempt, lookup)
		}
	}
}

func TestSemanticQueryTokensProjectBinderKindsDeterministically(t *testing.T) {
	session, tag := solveSemanticQueryFixture(t, semanticQuerySource)
	result, err := session.SemanticTokens(context.Background(), SemanticTokensRequest{Selector: selectorFor(tag), File: "main.lua"})
	if err != nil {
		t.Fatalf("SemanticTokens: %v", err)
	}
	if len(result.Tokens) == 0 {
		t.Fatal("SemanticTokens returned no solved binder tokens")
	}
	var hasVariable, hasParameter, hasFunction bool
	for _, item := range result.Tokens {
		if !item.Location.Valid() {
			t.Fatalf("semantic token = %#v, want a source location", item)
		}
		switch item.Kind {
		case SemanticTokenVariable:
			hasVariable = true
		case SemanticTokenParameter:
			hasParameter = true
		case SemanticTokenFunction:
			hasFunction = true
		}
	}
	if !hasVariable || !hasParameter || !hasFunction {
		t.Fatalf("semantic token kinds = %#v, want variable, parameter, and function", result.Tokens)
	}

	again, err := session.SemanticTokens(context.Background(), SemanticTokensRequest{Selector: selectorFor(tag), File: "main.lua"})
	if err != nil {
		t.Fatalf("SemanticTokens again: %v", err)
	}
	if !reflect.DeepEqual(result.Tokens, again.Tokens) {
		t.Fatalf("semantic tokens are not deterministic\nfirst=%#v\nsecond=%#v", result.Tokens, again.Tokens)
	}
}

func TestSemanticQueryTokensCarrySolvedPlacementModifier(t *testing.T) {
	source := "local scratch = { a = 1, b = 2 }\nlocal value = scratch.a + scratch.b\n"
	session := NewBatchSession()
	input := UnitInput{
		ID:         "semantic-token-placement",
		ModulePath: "example/semantic-token-placement",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{
			"main.lua": []byte(source),
		},
		StateLanes: enginestate.DefaultLanes(),
	}
	if _, err := session.UpsertUnit(context.Background(), input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	tag, err := session.EnsureSolved(context.Background(), SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	if err != nil {
		t.Fatalf("EnsureSolved: %v", err)
	}
	result, err := session.SemanticTokens(context.Background(), SemanticTokensRequest{Selector: selectorFor(tag), File: "main.lua"})
	if err != nil {
		t.Fatalf("SemanticTokens: %v", err)
	}
	for _, item := range result.Tokens {
		if semanticTokenHasModifier(item.Modifiers, SemanticTokenPlacement) {
			return
		}
	}
	t.Fatalf("semantic tokens = %#v, want a solved frame-local/decomposable placement token", result.Tokens)
}

func TestSemanticQueryTokensCarrySolvedTypestateModifier(t *testing.T) {
	const source = "local tx = {}\nbegin(tx)\n"
	lifecycle := manifest.New("lifecycle")
	if err := lifecycle.DefineTypestateProtocol(typestate.Definition{
		Protocol:    typestate.Protocol("transaction"),
		States:      []typestate.State{"active", "finished"},
		FinalStates: []typestate.State{"finished"},
		Transitions: []typestate.TransitionDecl{{From: "active", To: "finished"}},
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol: %v", err)
	}
	lifecycle.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().Param("tx", typ.Any).Build(),
		Effect: effect.Empty.With(lifecyclefx.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}),
	})
	session := NewBatchSession()
	input := UnitInput{
		ID:         "semantic-token-typestate",
		ModulePath: "example/semantic-token-typestate",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{
			"main.lua": []byte(source),
		},
		ExternalManifests: map[string]*manifest.Manifest{"lifecycle": lifecycle},
		Globals:           []string{"begin"},
		StateLanes:        enginestate.DefaultLanes(),
	}
	if _, err := session.UpsertUnit(context.Background(), input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	tag, err := session.EnsureSolved(context.Background(), SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	if err != nil {
		t.Fatalf("EnsureSolved: %v", err)
	}
	result, err := session.SemanticTokens(context.Background(), SemanticTokensRequest{Selector: selectorFor(tag), File: "main.lua"})
	if err != nil {
		t.Fatalf("SemanticTokens: %v", err)
	}
	for _, item := range result.Tokens {
		if item.Location.Span.StartLine == 1 && item.Location.Span.StartCol == 7 && semanticTokenHasModifier(item.Modifiers, SemanticTokenTypestateTracked) {
			return
		}
	}
	t.Fatalf("semantic tokens = %#v, want the lifecycle-tracked tx binder modifier", result.Tokens)
}

func TestSemanticQueryPositionLookupUsesOffsetBoundariesAndInnermostBody(t *testing.T) {
	session, tag := solveSemanticQueryFixture(t, semanticQuerySource)
	definition := strings.Index(semanticQuerySource, "localValue: number")
	if definition < 0 {
		t.Fatal("fixture localValue definition not found")
	}
	definition += len("local ")
	got, err := session.PositionLookup(context.Background(), PositionLookupRequest{
		Selector: selectorFor(tag),
		File:     "main.lua",
		Position: SourcePosition{Offset: definition},
	})
	if err != nil {
		t.Fatalf("PositionLookup definition: %v", err)
	}
	if !got.Found || got.Binder == nil || got.Binder.Name != "localValue" {
		t.Fatalf("PositionLookup definition = %#v, want localValue binder", got)
	}
	if got.Body.ID == "root" {
		t.Fatalf("PositionLookup body = %q, want enclosing nested function", got.Body.ID)
	}

	boundary, err := session.PositionLookup(context.Background(), PositionLookupRequest{
		Selector: selectorFor(tag),
		File:     "main.lua",
		Position: SourcePosition{Offset: definition + len("localValue")},
	})
	if err != nil {
		t.Fatalf("PositionLookup boundary: %v", err)
	}
	if boundary.Binder != nil {
		t.Fatalf("PositionLookup at exclusive end returned binder %#v", boundary.Binder)
	}

	read := strings.Index(semanticQuerySource, "captured + localValue")
	got, err = session.PositionLookup(context.Background(), PositionLookupRequest{
		Selector: selectorFor(tag),
		File:     "main.lua",
		Position: SourcePosition{Offset: read},
	})
	if err != nil {
		t.Fatalf("PositionLookup expression: %v", err)
	}
	if got.Expression == nil || got.Expression.Display == "" || got.Binder == nil || got.Binder.Name != "captured" {
		t.Fatalf("PositionLookup expression = %#v, want solved expression type and captured binder", got)
	}
}

func TestSemanticQueryDocumentSymbolsAndCallRelations(t *testing.T) {
	session, tag := solveSemanticQueryFixture(t, semanticQuerySource)
	symbols, err := session.DocumentSymbols(context.Background(), DocumentSymbolsRequest{Selector: selectorFor(tag), File: "main.lua"})
	if err != nil {
		t.Fatalf("DocumentSymbols: %v", err)
	}
	outer := documentSymbolNamed(symbols.Symbols, "outer", DocumentSymbolFunction)
	if outer == nil || !outer.Location.Valid() || documentSymbolNamed(outer.Children, "inner", DocumentSymbolFunction) == nil {
		t.Fatalf("document symbols = %#v, want nested outer/inner function tree", symbols.Symbols)
	}
	field := documentSymbolNamed(symbols.Symbols, "outer", DocumentSymbolModuleField)
	if field == nil || field.Anchor == "" || !field.Location.Valid() {
		t.Fatalf("document symbols = %#v, want stable outer module field", symbols.Symbols)
	}

	calls, err := session.CallRelations(context.Background(), CallRelationsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("CallRelations: %v", err)
	}
	if !hasResolvedCall(calls.Bodies, "outer") || !hasResolvedCall(calls.Bodies, "inner") {
		t.Fatalf("call relations = %#v, want resolved local outer and inner calls", calls.Bodies)
	}
}

func TestSemanticQueryRepairsOnlyFollowDeclaredDescriptors(t *testing.T) {
	source := `local value: number = 1
local redundant = value as number
return redundant
`
	session, tag := solveSemanticQueryFixture(t, source)
	result, err := session.RepairActions(context.Background(), RepairActionsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("RepairActions: %v", err)
	}
	if len(result.Actions) == 0 {
		t.Fatal("RepairActions returned no descriptor-declared actions")
	}
	declared := declaredRepairKinds()
	for _, action := range result.Actions {
		if _, ok := declared[action.Kind]; !ok || !action.Target.Valid() || action.Code == "" {
			t.Fatalf("repair action = %#v, want a declared kind and target", action)
		}
	}
	if !hasRepairKind(result.Actions, judgment.RepairRemoveRedundantClaim) {
		t.Fatalf("repair actions = %#v, want redundant-claim action", result.Actions)
	}
	for _, action := range result.Actions {
		if action.Kind != judgment.RepairRemoveRedundantClaim {
			continue
		}
		if len(action.Payload.Edits) != 1 || action.Payload.Edits[0].NewText != "value" {
			t.Fatalf("redundant-claim repair payload = %#v, want exact operand replacement", action.Payload)
		}
	}
	nilGuard := repairActionsFromJudgments("main.lua", []judgment.Judgment{{
		Code:     judgment.CodeCallCallee,
		Spans:    []judgment.SpanRef{{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}},
		Evidence: judgment.EvidenceChain{{Detail: judgment.CalleeMayBeNilEvidenceDetail(true)}},
	}})
	if !hasRepairKind(nilGuard, judgment.RepairAddNilGuard) {
		t.Fatalf("nil cause repairs = %#v, want nil-guard action", nilGuard)
	}
	annotation := repairActionsFromJudgments("main.lua", []judgment.Judgment{{
		Code:     judgment.CodeAssignment,
		Expected: judgment.TypeRef{Type: typ.Number},
		Spans:    []judgment.SpanRef{{File: "main.lua", StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}},
	}})
	if len(annotation) != 1 || annotation[0].Kind != judgment.RepairAddAnnotation || annotation[0].Payload.Type == "" {
		t.Fatalf("annotation repairs = %#v, want structured annotation payload", annotation)
	}
	fixedShape := repairActionsFromJudgments("main.lua", []judgment.Judgment{{
		Code:  judgment.CodeAdviceShapePolymorphic,
		Spans: []judgment.SpanRef{{File: "main.lua", StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 4}},
		Evidence: judgment.EvidenceChain{
			{Detail: judgment.AdviceShapeConditionalFieldEvidenceDetail("delta", "content")},
			{Detail: judgment.AdviceShapeConditionalFieldEvidenceDetail("delta", "args")},
			{Detail: judgment.AdviceShapeConditionalFieldEvidenceDetail("delta", "content")},
		},
	}})
	if len(fixedShape) != 1 || fixedShape[0].Kind != judgment.RepairConstructFixedShape || !reflect.DeepEqual(fixedShape[0].Payload.Fields, []string{"args", "content"}) {
		t.Fatalf("fixed-shape repairs = %#v, want one atomic fixed-shape action naming sorted union fields", fixedShape)
	}
}

func TestSemanticQueryRepeatedSolvesAreDeterministic(t *testing.T) {
	session, first := solveSemanticQueryFixture(t, semanticQuerySource)
	second, err := session.EnsureSolved(context.Background(), SolveRequest{UnitID: "semantic-query", Freshness: FreshnessRequireNew})
	if err != nil {
		t.Fatalf("second EnsureSolved: %v", err)
	}
	firstBinders, err := session.BinderOccurrences(context.Background(), BinderOccurrencesRequest{Selector: selectorFor(first)})
	if err != nil {
		t.Fatal(err)
	}
	secondBinders, err := session.BinderOccurrences(context.Background(), BinderOccurrencesRequest{Selector: selectorFor(second)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstBinders.Binders, secondBinders.Binders) {
		t.Fatalf("binder projections differ across identical solves\nfirst=%#v\nsecond=%#v", firstBinders.Binders, secondBinders.Binders)
	}
	firstCalls, err := session.CallRelations(context.Background(), CallRelationsRequest{Selector: selectorFor(first)})
	if err != nil {
		t.Fatal(err)
	}
	secondCalls, err := session.CallRelations(context.Background(), CallRelationsRequest{Selector: selectorFor(second)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstCalls.Bodies, secondCalls.Bodies) {
		t.Fatalf("call projections differ across identical solves\nfirst=%#v\nsecond=%#v", firstCalls.Bodies, secondCalls.Bodies)
	}
}

func binderNamed(items []BinderInfo, name string) *BinderInfo {
	for index := range items {
		if items[index].Name == name {
			return &items[index]
		}
	}
	return nil
}

func countOccurrenceRole(items []BinderOccurrence, role BinderOccurrenceRole) int {
	count := 0
	for _, item := range items {
		if item.Role == role {
			count++
		}
	}
	return count
}
func documentSymbolNamed(items []DocumentSymbol, name string, kind DocumentSymbolKind) *DocumentSymbol {
	for index := range items {
		if items[index].Name == name && items[index].Kind == kind {
			return &items[index]
		}
	}
	return nil
}
func hasResolvedCall(items []BodyCallRelations, name string) bool {
	for _, body := range items {
		for _, call := range body.Calls {
			if call.Callee != nil && call.Callee.Name == name {
				return true
			}
		}
	}
	return false
}
func hasRepairKind(items []RepairAction, kind judgment.RepairKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}
func declaredRepairKinds() map[judgment.RepairKind]struct{} {
	out := map[judgment.RepairKind]struct{}{}
	registry := judgment.DefaultRegistry()
	for _, code := range registry.Codes() {
		spec, _ := registry.Lookup(code)
		for _, repair := range spec.Repairs {
			out[repair.Kind] = struct{}{}
		}
	}
	return out
}
