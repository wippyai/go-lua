package service

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	checkdiagnostics "github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/embedding"
	enginestate "github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestBatchSessionPublishesCompleteQueryableResult(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := UnitInput{
		ID:         "unit-a",
		ModulePath: "example/a",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{
			"main.lua": []byte(`
type Config = { limit: number }
local clean: Config = { limit = 3 }
local hoisted = 0
local i = 0
while i < 3 do
	hoisted = hoisted + clean.limit
	i = i + 1
end
function build(ids: {string}): {items: {[string]: {id: string}}, count: number}
	local batch: {items: {[string]: {id: string}}, count: number} = {items = {}, count = 0}
	for _, id in ipairs(ids) do
		batch.count = batch.count + 1
		local item: {id: string} = {id = id}
		batch.items[id] = item
	end
	return batch
end
local function identity(value: number): number
	return value
end
local wrong = missing_value
local exported = { value = identity(1), built = build({"a"}) }
return exported
`),
		},
		Profile:    "typed",
		StateLanes: enginestate.DefaultLanes(),
	}
	state, err := session.UpsertUnit(ctx, input)
	if err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	if state.UnitDigest.IsZero() || state.SourceDigests[embedding.FileDocument("main.lua")].IsZero() || !state.Changed {
		t.Fatalf("unit state = %#v, want new content digests", state)
	}

	tag, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID, Trigger: TriggerBatch})
	if err != nil {
		t.Fatalf("EnsureSolved: %v", err)
	}
	if tag.SolveSeq == 0 || tag.UnitDigest != state.UnitDigest || tag.ManifestDigest.IsZero() {
		t.Fatalf("result tag = %#v, want complete digest tag", tag)
	}
	if len(tag.BodyInputDigests) < 2 || tag.BodyInputDigests["root"] == 0 {
		t.Fatalf("body input digests = %#v, want root and nested function", tag.BodyInputDigests)
	}

	completed, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(tag)})
	if !ok || !completed.Valid() {
		t.Fatal("LastComplete did not return published result")
	}
	items := completed.Judgments()
	if len(items) == 0 {
		t.Fatal("completed result has no raw judgments")
	}
	for _, item := range items {
		if item.Subject.Anchor.IsZero() {
			t.Fatalf("judgment %s has no subject anchor", item.Code)
		}
		if item.BodyInputDigest == 0 || !containsBodyInputDigest(tag.BodyInputDigests, item.BodyInputDigest) {
			t.Fatalf("judgment %s BodyInputDigest = %d, body input digests = %#v", item.Code, item.BodyInputDigest, tag.BodyInputDigests)
		}
	}
	if len(completed.RenderedDiagnostics()) == 0 {
		t.Fatalf("completed result has no rendered diagnostics; judgments = %#v", items)
	}
	manifestPath, manifestDigest, manifestData := completed.ManifestBytes()
	if manifestPath != input.ModulePath || manifestDigest != digestBytes(manifestData) || len(manifestData) == 0 {
		t.Fatalf("manifest = path %q digest %s bytes %d", manifestPath, manifestDigest, len(manifestData))
	}
	if _, err := manifest.Decode(manifestData); err != nil {
		t.Fatalf("manifest decode: %v", err)
	}
	plan := completed.PlacementPlan()
	if len(plan.Entries) == 0 {
		t.Fatalf("placement plan = %#v, want allocation entries", plan)
	}
	if entries := completed.SummarySnapshot().Entries(); len(entries) == 0 {
		t.Fatal("completed result has no summary entries")
	}

	list, err := session.ListJudgments(ctx, ListJudgmentsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("ListJudgments: %v", err)
	}
	if len(list.Judgments) != len(items) || len(list.CodeSpecs) == 0 || list.Meta.Tag.SolveSeq != tag.SolveSeq {
		t.Fatalf("ListJudgments = %#v, want all completed judgments", list)
	}
	anchorKey := list.Judgments[0].Subject.Anchor.StableKey()
	anchored, err := session.JudgmentsByAnchor(ctx, JudgmentsByAnchorRequest{
		Selector:  selectorFor(tag),
		AnchorKey: anchorKey,
	})
	if err != nil {
		t.Fatalf("JudgmentsByAnchor: %v", err)
	}
	if len(anchored.Judgments) == 0 || anchored.Anchor.StableKey() != anchorKey {
		t.Fatalf("JudgmentsByAnchor = %#v, want %q", anchored, anchorKey)
	}
	diagnosticsResult, err := session.Diagnostics(ctx, ListDiagnosticsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("Diagnostics: %v", err)
	}
	if len(diagnosticsResult.Raw) != len(items) || len(diagnosticsResult.Rendered) == 0 {
		t.Fatalf("Diagnostics raw/rendered = %d/%d", len(diagnosticsResult.Raw), len(diagnosticsResult.Rendered))
	}
	withOptIn, err := session.Diagnostics(ctx, ListDiagnosticsRequest{
		Selector: selectorFor(tag),
		DiagnosticPolicy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
			checkdiagnostics.CodeUnusedLocal: diagnostic.Enable(),
		}},
	})
	if err != nil {
		t.Fatalf("Diagnostics with policy: %v", err)
	}
	if len(withOptIn.Raw) != len(items) || len(withOptIn.Rendered) <= len(diagnosticsResult.Rendered) {
		t.Fatalf("policy diagnostics raw/rendered = %d/%d, default rendered = %d", len(withOptIn.Raw), len(withOptIn.Rendered), len(diagnosticsResult.Rendered))
	}
	manifestResult, err := session.ManifestBytes(ctx, ExportManifestRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("ManifestBytes: %v", err)
	}
	if manifestResult.Digest != tag.ManifestDigest || !bytes.Equal(manifestResult.Data, manifestData) {
		t.Fatalf("manifest query = %#v, want completed manifest", manifestResult)
	}
	placementResult, err := session.PlacementPlan(ctx, PlacementPlanRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("PlacementPlan: %v", err)
	}
	if !reflect.DeepEqual(placementResult.Plan, plan) {
		t.Fatalf("placement query = %#v, want %#v", placementResult.Plan, plan)
	}
	summaries, err := session.SummarySnapshot(ctx, SummarySnapshotRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("SummarySnapshot: %v", err)
	}
	if len(summaries.Summaries) == 0 || len(summaries.Digests) != len(summaries.Summaries) {
		t.Fatalf("summary query entries/digests = %d/%d", len(summaries.Summaries), len(summaries.Digests))
	}
	versions, err := session.BodyInputDigests(ctx, BodyInputDigestsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("BodyInputDigests: %v", err)
	}
	if !reflect.DeepEqual(versions.Digests, tag.BodyInputDigests) {
		t.Fatalf("body input digest query = %#v, want %#v", versions.Digests, tag.BodyInputDigests)
	}

	assertCompletedResultIsDefensivelyImmutable(t, completed)
}

func TestDocumentKeyedUnitInputBindsResultLocationsAndPreservesDisplay(t *testing.T) {
	ctx := context.Background()
	document := embedding.RegistryDocument("component:orders/source:lua")
	content := []byte("local value = missing_value\n")
	input := UnitInput{
		ID:            "registry-orders",
		ModulePath:    "registry/orders",
		EntryDocument: document,
		Sources: map[embedding.DocumentID]embedding.SourceSnapshot{
			document: {Document: document, ProviderRevision: "registry-42", Content: content},
		},
		DocumentLabels: embedding.StaticLabels{document: "orders.lua"},
		Plan: embedding.UnitPlan{
			ID:      "registry-orders",
			Entry:   document,
			Sources: []embedding.DocumentID{document},
			Imports: []embedding.UnitImport{{Alias: "shared", TargetUnit: "registry-shared", ManifestDigest: digestBytes([]byte("shared-manifest"))}},
		},
		ResolutionDigest: digestBytes([]byte("workspace-view-42")),
	}
	session := NewBatchSession()
	state, err := session.UpsertUnit(ctx, input)
	if err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	digest := state.SourceDigests[document]
	if digest.IsZero() || digest != digestBytes(content) {
		t.Fatalf("source digest = %s, want digest of materialized content", digest)
	}
	tag := mustSolve(t, session, SolveRequest{UnitID: input.ID})
	if tag.SourceDigests[document] != digest {
		t.Fatalf("result tag source digests = %#v, want registry document key", tag.SourceDigests)
	}
	completed, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(tag)})
	if !ok {
		t.Fatal("completed result missing")
	}
	for _, item := range completed.Judgments() {
		for _, span := range item.Spans {
			if span.Location.Document != document || span.Location.ContentDigest != digest || !span.Location.Valid() {
				t.Fatalf("judgment span location = %#v, want digest-bound registry location", span.Location)
			}
			if span.File != "orders.lua" {
				t.Fatalf("judgment span display = %q, want label projection", span.File)
			}
		}
	}
	for _, item := range completed.RenderedDiagnostics() {
		if item.Location.Document != document || item.Location.ContentDigest != digest || !item.Location.Valid() {
			t.Fatalf("diagnostic location = %#v, want digest-bound registry location", item.Location)
		}
		if item.Position.File != "orders.lua" {
			t.Fatalf("diagnostic display = %q, want label projection", item.Position.File)
		}
	}
}

func TestFileCompatibilityConstructorPreservesDiagnosticRenderBytes(t *testing.T) {
	ctx := context.Background()
	content := []byte("local value = missing_value\n")
	legacy := UnitInput{
		ID:          "legacy",
		ModulePath:  "example/legacy",
		EntryFile:   "main.lua",
		SourceFiles: map[string][]byte{"main.lua": content},
	}
	modern := NewUnitInputFromFiles("modern", "example/legacy", "main.lua", map[string][]byte{"main.lua": content})
	legacySession := NewBatchSession()
	modernSession := NewBatchSession()
	if _, err := legacySession.UpsertUnit(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := modernSession.UpsertUnit(ctx, modern); err != nil {
		t.Fatal(err)
	}
	legacyTag := mustSolve(t, legacySession, SolveRequest{UnitID: legacy.ID})
	modernTag := mustSolve(t, modernSession, SolveRequest{UnitID: modern.ID})
	legacyResult, _ := legacySession.LastComplete(ctx, ResultRequest{Selector: selectorFor(legacyTag)})
	modernResult, _ := modernSession.LastComplete(ctx, ResultRequest{Selector: selectorFor(modernTag)})
	legacyDiagnostic := legacyResult.RenderedDiagnostics()[0]
	modernDiagnostic := modernResult.RenderedDiagnostics()[0]
	options := diagnostic.RenderOptions{Sources: diagnostic.SourceMap{"main.lua": string(content)}}
	if got, want := diagnostic.Render(modernDiagnostic, options), diagnostic.Render(legacyDiagnostic, options); got != want {
		t.Fatalf("file compatibility changed render bytes\nwant:\n%s\ngot:\n%s", want, got)
	}
	if modernDiagnostic.Location.Document != embedding.FileDocument("main.lua") {
		t.Fatalf("modern file location = %#v, want file document id", modernDiagnostic.Location)
	}
}

func TestBatchSessionDeterminismAndDigestScopedInvalidation(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	unitA := UnitInput{
		ID:         "unit-a",
		ModulePath: "example/a",
		EntryFile:  "a.lua",
		SourceFiles: map[string][]byte{"a.lua": []byte(`local value: number = 1
return value
`)},
		Profile: "typed",
	}
	if _, err := session.UpsertUnit(ctx, unitA); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	a1 := mustSolve(t, session, SolveRequest{UnitID: unitA.ID})
	a2 := mustSolve(t, session, SolveRequest{UnitID: unitA.ID, Freshness: FreshnessRequireNew})
	if a2.SolveSeq <= a1.SolveSeq {
		t.Fatalf("forced solve seq = %d, want greater than %d", a2.SolveSeq, a1.SolveSeq)
	}
	assertStableResultContent(t, a1, a2)

	state, err := session.UpsertUnit(ctx, unitA)
	if err != nil {
		t.Fatalf("identical upsert A: %v", err)
	}
	if state.Changed {
		t.Fatal("identical input marked changed")
	}
	cached := mustSolve(t, session, SolveRequest{UnitID: unitA.ID})
	if cached.SolveSeq != a2.SolveSeq {
		t.Fatalf("cached solve seq = %d, want %d", cached.SolveSeq, a2.SolveSeq)
	}

	unitB := UnitInput{
		ID:         "unit-b",
		ModulePath: "example/b",
		EntryFile:  "b.lua",
		SourceFiles: map[string][]byte{"b.lua": []byte(`local value: number = 2
return value
`)},
		Profile: "typed",
	}
	if _, err := session.UpsertUnit(ctx, unitB); err != nil {
		t.Fatalf("upsert B: %v", err)
	}
	b1 := mustSolve(t, session, SolveRequest{UnitID: unitB.ID})

	unitB.SourceFiles["b.lua"] = append(unitB.SourceFiles["b.lua"], []byte("-- comment-only edit\n")...)
	bState, err := session.UpsertUnit(ctx, unitB)
	if err != nil {
		t.Fatalf("edited upsert B: %v", err)
	}
	if !bState.Changed || bState.UnitDigest == b1.UnitDigest || bState.SourceDigests[embedding.FileDocument("b.lua")] == b1.SourceDigests[embedding.FileDocument("b.lua")] {
		t.Fatalf("edited B state = %#v, previous tag = %#v", bState, b1)
	}
	staleB, err := session.PlacementPlan(ctx, PlacementPlanRequest{Selector: ResultSelector{UnitID: unitB.ID, Profile: unitB.Profile}})
	if err != nil {
		t.Fatalf("stale B query: %v", err)
	}
	if !staleB.Meta.Stale {
		t.Fatal("last complete B result was not marked stale after edit")
	}
	b2 := mustSolve(t, session, SolveRequest{UnitID: unitB.ID})
	if b2.UnitDigest == b1.UnitDigest || b2.SourceDigests[embedding.FileDocument("b.lua")] == b1.SourceDigests[embedding.FileDocument("b.lua")] {
		t.Fatalf("B source edit reused unit/source digest: before=%#v after=%#v", b1, b2)
	}
	if b2.ManifestDigest != b1.ManifestDigest || !reflect.DeepEqual(b2.BodyInputDigests, b1.BodyInputDigests) {
		t.Fatalf("comment-only edit changed semantic digests: before=%#v after=%#v", b1, b2)
	}

	aLatest, ok := session.LastComplete(ctx, ResultRequest{Selector: ResultSelector{UnitID: unitA.ID, Profile: unitA.Profile}})
	if !ok {
		t.Fatal("unit A disappeared after editing unit B")
	}
	if got := aLatest.Tag(); got.SolveSeq != a2.SolveSeq || got.UnitDigest != a2.UnitDigest {
		t.Fatalf("editing B invalidated A: got %#v, want %#v", got, a2)
	}
	aQuery, err := session.ManifestBytes(ctx, ExportManifestRequest{Selector: selectorFor(a2)})
	if err != nil {
		t.Fatalf("query A after editing B: %v", err)
	}
	if aQuery.Meta.Stale {
		t.Fatal("unit A result marked stale after editing unit B")
	}
}

func TestBatchSessionReusesEquivalentAnalysisAndInvalidatesChangedInput(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	base := UnitInput{
		ID:         "analysis-cache-a",
		ModulePath: "example/analysis-cache",
		EntryFile:  "shared.lua",
		SourceFiles: map[string][]byte{"shared.lua": []byte(`local value: number = 1
return value
`)},
		Profile: "typed",
	}
	if _, err := session.UpsertUnit(ctx, base); err != nil {
		t.Fatalf("UpsertUnit A: %v", err)
	}
	first := mustSolve(t, session, SolveRequest{UnitID: base.ID, Freshness: FreshnessRequireNew})

	// UnitID is intentionally not part of UnitDigest. A second independently
	// resolved unit with the same materialized source and presentation label may
	// reuse the immutable body/summary result, but gets its own publication tag.
	copyInput := base
	copyInput.ID = "analysis-cache-b"
	if _, err := session.UpsertUnit(ctx, copyInput); err != nil {
		t.Fatalf("UpsertUnit B: %v", err)
	}
	second := mustSolve(t, session, SolveRequest{UnitID: copyInput.ID, Freshness: FreshnessRequireNew})
	if second.SolveSeq == first.SolveSeq || second.UnitID != copyInput.ID || second.UnitDigest != first.UnitDigest {
		t.Fatalf("reused publication tag = %#v, first = %#v", second, first)
	}
	session.mu.RLock()
	cacheEntries := len(session.analysisCache)
	session.mu.RUnlock()
	if cacheEntries != 1 {
		t.Fatalf("analysis cache entries = %d, want one digest-scoped shared result", cacheEntries)
	}

	changed := copyInput
	changed.SourceFiles = map[string][]byte{"shared.lua": []byte(`local value: string = "changed"
return value
`)}
	if _, err := session.UpsertUnit(ctx, changed); err != nil {
		t.Fatalf("UpsertUnit changed: %v", err)
	}
	third := mustSolve(t, session, SolveRequest{UnitID: changed.ID, Freshness: FreshnessRequireNew})
	if third.UnitDigest == second.UnitDigest || third.BodyInputDigests["root"] == second.BodyInputDigests["root"] {
		t.Fatalf("changed input reused stale analysis: before=%#v after=%#v", second, third)
	}
	session.mu.RLock()
	cacheEntries = len(session.analysisCache)
	session.mu.RUnlock()
	if cacheEntries != 2 {
		t.Fatalf("analysis cache entries after content change = %d, want two digest versions", cacheEntries)
	}
}

func TestBatchSessionHighFanoutMaterializedContextsAreDeterministic(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := UnitInput{
		ID:         "high-fanout-contexts",
		ModulePath: "example/high-fanout-contexts",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{"main.lua": []byte(`
local function alpha(value: number): number
	return value + 1
end

local function beta(value: number): number
	return value * 2
end

local function gamma(value: number): number
	return value - 1
end

local function first(): number
	return alpha(1) + alpha(2) + beta(3) + beta(4) + gamma(5) + gamma(6)
end

local function second(): number
	return alpha(7) + alpha(8) + beta(9) + beta(10) + gamma(11) + gamma(12)
end

local function third(): number
	return alpha(13) + alpha(14) + beta(15) + beta(16) + gamma(17) + gamma(18)
end

return first() + second() + third()
`)},
		Profile: "typed",
	}
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}

	firstTag := mustSolve(t, session, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	first, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(firstTag)})
	if !ok {
		t.Fatal("first completed result missing")
	}
	wantBodies := first.Bodies()
	if len(wantBodies) < 24 {
		t.Fatalf("bodies = %d, want high-fanout materialized contexts", len(wantBodies))
	}

	for run := 1; run < 12; run++ {
		tag := mustSolve(t, session, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
		assertStableResultContent(t, firstTag, tag)
		completed, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(tag)})
		if !ok {
			t.Fatalf("run %d completed result missing", run)
		}
		if got := completed.Bodies(); !reflect.DeepEqual(got, wantBodies) {
			t.Fatalf("run %d body ordering or ResultVersions changed\nwant: %#v\n got: %#v", run, wantBodies, got)
		}
	}
}

func unencodableManifest() *manifest.Manifest {
	m := manifest.New("example/bad-iterator")
	m.DefineFunctionSignature("iter", signature.Function{
		Type: typ.Func().
			Param("input", typ.NewArray(typ.String)).
			Build(),
		Effect: effect.Empty.With(iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IteratorKind(99),
		}),
	})
	return m
}

func TestUnitInputDigestPropagatesManifestEncodeError(t *testing.T) {
	input := UnitInput{
		ID:                "unit-a",
		ModulePath:        "example/a",
		EntryDocument:     embedding.FileDocument("main.lua"),
		ExternalManifests: map[string]*manifest.Manifest{"bad.lua": unencodableManifest()},
	}
	digest, err := unitInputDigest(input, map[embedding.DocumentID]Digest{embedding.FileDocument("main.lua"): digestBytes([]byte("x"))})
	if err == nil {
		t.Fatal("unitInputDigest: want error for unencodable external manifest, got nil")
	}
	if !strings.Contains(err.Error(), "unknown iterator kind 99") {
		t.Fatalf("unitInputDigest error = %v, want unknown iterator kind", err)
	}
	if digest != (Digest{}) {
		t.Fatalf("unitInputDigest digest = %v, want zero digest alongside error", digest)
	}
}

func TestUpsertUnitRejectsUnencodableExternalManifest(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := UnitInput{
		ID:         "unit-a",
		ModulePath: "example/a",
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{"main.lua": []byte(`local value: number = 1
return value
`)},
		ExternalManifests: map[string]*manifest.Manifest{"bad.lua": unencodableManifest()},
		Profile:           "typed",
	}
	if _, err := session.UpsertUnit(ctx, input); err == nil {
		t.Fatal("UpsertUnit: want error for unencodable external manifest, got nil")
	}
	if _, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID, Trigger: TriggerBatch}); !errors.Is(err, ErrUnitNotFound) {
		t.Fatalf("EnsureSolved after rejected upsert: err = %v, want %v", err, ErrUnitNotFound)
	}
}

func TestBatchSessionCancellationDropsHeavySolveWithoutPublishing(t *testing.T) {
	session := NewBatchSession()
	input := heavySolveInput("cancel-heavy")
	if _, err := session.UpsertUnit(context.Background(), input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		tag ResultTag
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		tag, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
		done <- outcome{tag: tag, err: err}
	}()

	// The generated program keeps the transfer/summary worklists active well
	// beyond this handoff; cancellation therefore exercises an in-flight solve,
	// not only the preflight context check.
	time.Sleep(10 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("EnsureSolved error = %v, want context cancellation", got.err)
		}
		if got.tag.SolveSeq != 0 || !got.tag.UnitDigest.IsZero() || len(got.tag.SourceDigests) != 0 {
			t.Fatalf("EnsureSolved tag = %#v, want no canceled result", got.tag)
		}
		if elapsed := time.Since(start); elapsed >= time.Second {
			t.Fatalf("EnsureSolved cancellation took %s, want <1s", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("EnsureSolved did not return within 1s of cancellation")
	}
	if _, ok := session.LastComplete(context.Background(), ResultRequest{Selector: ResultSelector{UnitID: input.ID}}); ok {
		t.Fatal("canceled solve published a completed result")
	}
}

func TestBatchSessionDiscardsSnapshotAfterInterleavedUnitEdit(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := heavySolveInput("discard-heavy")
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}

	unit, profile, documentVersion, _, cached, err := session.solveInputSnapshot(SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	if err != nil || cached {
		t.Fatalf("solveInputSnapshot = cached=%v err=%v", cached, err)
	}
	done := make(chan *completedSnapshot, 1)
	errs := make(chan error, 1)
	go func() {
		snapshot, err := solveUnit(ctx, unit, profile, documentVersion)
		if err != nil {
			errs <- err
			return
		}
		done <- snapshot
	}()

	// This edit races an outside-lock solve. Whether it lands immediately before
	// or during transfer, publication must reject the old retained generation.
	edited := input
	edited.SourceFiles = cloneSourceFiles(input.SourceFiles)
	edited.SourceFiles[input.EntryFile] = append(edited.SourceFiles[input.EntryFile], []byte("\n-- edited while solve was in flight\n")...)
	if _, err := session.UpsertUnit(ctx, edited); err != nil {
		t.Fatalf("edited UpsertUnit: %v", err)
	}

	var snapshot *completedSnapshot
	select {
	case err := <-errs:
		t.Fatalf("solveUnit: %v", err)
	case snapshot = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("old solve did not complete")
	}
	if tag, discard, err := session.publishSolved(input.ID, unit, profile, snapshot); err != nil || !discard || tag.SolveSeq != 0 || !tag.UnitDigest.IsZero() || len(tag.SourceDigests) != 0 {
		t.Fatalf("publish old snapshot = tag=%#v discard=%v err=%v, want discard", tag, discard, err)
	}
	if _, ok := session.LastComplete(ctx, ResultRequest{Selector: ResultSelector{UnitID: input.ID}}); ok {
		t.Fatal("discarded snapshot became queryable")
	}
	updated := mustSolve(t, session, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	if updated.UnitDigest == unit.digest {
		t.Fatalf("re-solve published old digest %s", updated.UnitDigest)
	}
}

func TestBatchSessionConcurrentEnsureSolvedCallsAreBothValid(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := serviceStressInput("same-unit")
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}

	type outcome struct {
		tag ResultTag
		err error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			tag, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
			outcomes <- outcome{tag: tag, err: err}
		}()
	}
	first := <-outcomes
	second := <-outcomes
	if first.err != nil || second.err != nil {
		t.Fatalf("EnsureSolved errors = %v / %v", first.err, second.err)
	}
	if first.tag.SolveSeq == second.tag.SolveSeq || first.tag.UnitDigest != second.tag.UnitDigest || first.tag.UnitDigest.IsZero() {
		t.Fatalf("concurrent tags = %#v / %#v, want distinct complete same-input results", first.tag, second.tag)
	}
}

func TestBatchSessionConcurrentSolveQueryUpsertRemove(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	primary := serviceStressInput("primary")
	secondary := serviceStressInput("secondary")
	if _, err := session.UpsertUnit(ctx, primary); err != nil {
		t.Fatalf("upsert primary: %v", err)
	}
	if _, err := session.UpsertUnit(ctx, secondary); err != nil {
		t.Fatalf("upsert secondary: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	report := func(err error) {
		if err != nil {
			errs <- err
		}
	}
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				_, err := session.EnsureSolved(ctx, SolveRequest{UnitID: primary.ID, Freshness: FreshnessRequireNew})
				report(err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 16; i++ {
			updated := secondary
			updated.SourceFiles = cloneSourceFiles(secondary.SourceFiles)
			updated.SourceFiles[updated.EntryFile] = append(updated.SourceFiles[updated.EntryFile], []byte("\n-- update "+strconv.Itoa(i))...)
			report(func() error { _, err := session.UpsertUnit(ctx, updated); return err }())
			if i%3 == 0 {
				report(session.RemoveUnit(ctx, secondary.ID))
				report(func() error { _, err := session.UpsertUnit(ctx, secondary); return err }())
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 64; i++ {
			completed, ok := session.LastComplete(ctx, ResultRequest{Selector: ResultSelector{UnitID: primary.ID}})
			if !ok {
				continue
			}
			if !completed.Valid() {
				report(errors.New("LastComplete returned invalid completed result"))
				continue
			}
			_, err := session.ListJudgments(ctx, ListJudgmentsRequest{Selector: ResultSelector{UnitID: primary.ID}})
			report(err)
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent service operation: %v", err)
		}
	}
}

func serviceStressInput(id UnitID) UnitInput {
	return UnitInput{
		ID:         id,
		ModulePath: "example/" + string(id),
		EntryFile:  "main.lua",
		SourceFiles: map[string][]byte{"main.lua": []byte(`
local function increment(value: number): number
	return value + 1
end
return increment(41)
`)},
		Profile: "typed",
	}
}

func heavySolveInput(id UnitID) UnitInput {
	var source strings.Builder
	for i := 0; i < 128; i++ {
		name := strconv.Itoa(i)
		source.WriteString("local function heavy")
		source.WriteString(name)
		source.WriteString("(value: number): number\n")
		source.WriteString("\tlocal total = value\n\tfor n = 1, 128 do\n\t\ttotal = total + n\n\tend\n\treturn total\nend\n")
	}
	source.WriteString("return ")
	for i := 0; i < 128; i++ {
		if i != 0 {
			source.WriteString(" + ")
		}
		source.WriteString("heavy")
		source.WriteString(strconv.Itoa(i))
		source.WriteString("(1)")
	}
	source.WriteString("\n")
	input := serviceStressInput(id)
	input.SourceFiles[input.EntryFile] = []byte(source.String())
	return input
}

func assertCompletedResultIsDefensivelyImmutable(t *testing.T, completed CompletedResult) {
	t.Helper()
	tag := completed.Tag()
	mainDocument := embedding.FileDocument("main.lua")
	originalSource := tag.SourceDigests[mainDocument]
	tag.SourceDigests[mainDocument] = Digest{}
	tag.BodyInputDigests["root"] = 0
	if again := completed.Tag(); again.SourceDigests[mainDocument] != originalSource || again.BodyInputDigests["root"] == 0 {
		t.Fatal("mutating returned tag changed completed snapshot")
	}
	items := completed.Judgments()
	originalCode := items[0].Code
	items[0].Code = judgment.Code("mutated")
	if completed.Judgments()[0].Code != originalCode {
		t.Fatal("mutating returned judgments changed completed snapshot")
	}
	_, _, data := completed.ManifestBytes()
	originalByte := data[0]
	data[0]++
	_, _, again := completed.ManifestBytes()
	if again[0] != originalByte {
		t.Fatal("mutating returned manifest bytes changed completed snapshot")
	}
	plan := completed.PlacementPlan()
	if len(plan.HoistableLoads) == 0 || len(plan.HoistableLoads[0].ReadPath.Segments) == 0 {
		t.Fatalf("fixture has no hoistable load with a read-path segment: %#v", plan.HoistableLoads)
	}
	originalLoad := plan.HoistableLoads[0]
	originalLoad.ReadPath = originalLoad.ReadPath.Clone()
	plan.HoistableLoads[0].BodyID++
	plan.HoistableLoads[0].ReadPath.Segments[0].Name = "mutated"
	againPlan := completed.PlacementPlan()
	if !reflect.DeepEqual(againPlan.HoistableLoads[0], originalLoad) {
		t.Fatal("mutating a hoistable load or its read-path segments changed the completed snapshot")
	}

	assertSummaryAccessorsAreDefensivelyImmutable(t, completed)
}

// assertSummaryAccessorsAreDefensivelyImmutable mutates every owned field a
// client can reach through SummaryView (Returns, HeapTableObjects,
// NormalReturnFacts-family slices) via both Read and Entries, then confirms
// the published snapshot is unaffected. This guards against SummaryView ever
// exposing summary.Snapshot's zero-copy ReadOwnedNormalized/
// EntriesOwnedNormalized reads.
func assertSummaryAccessorsAreDefensivelyImmutable(t *testing.T, completed CompletedResult) {
	t.Helper()
	entries := completed.SummarySnapshot().Entries()
	if len(entries) == 0 {
		t.Fatal("completed result has no summary entries")
	}

	// A sentinel drawn from a real Returns fact. Assigning it into an
	// unrelated slot and checking the slot reverts on refetch works
	// regardless of whether that slot's own natural value happens to be the
	// product.Value zero value.
	var sentinel product.Value
	for _, entry := range entries {
		if len(entry.Summary.Returns) != 0 && entry.Summary.Returns[0] != (product.Value{}) {
			sentinel = entry.Summary.Returns[0]
			break
		}
	}
	if sentinel == (product.Value{}) {
		t.Fatal("no summary entry carries a non-zero Returns value to use as a mutation sentinel")
	}

	var sawReturns, sawHeapTableObjects, sawNormalReturnParams bool
	for _, entry := range entries {
		key := entry.Key

		if len(entry.Summary.Returns) != 0 {
			sawReturns = true
			assertMutatingSlotIsIsolated(t, completed, key, sentinel,
				func(s summary.Summary) []product.Value { return s.Returns })
		}
		if len(entry.Summary.HeapTableObjects) != 0 {
			sawHeapTableObjects = true
			assertMutatingHeapTableObjectsIsIsolated(t, completed, key)
		}
		if len(entry.Summary.NormalReturnParams) != 0 {
			sawNormalReturnParams = true
			assertMutatingSlotIsIsolated(t, completed, key, sentinel,
				func(s summary.Summary) []product.Value { return s.NormalReturnParams })
		}
	}
	if !sawReturns {
		t.Fatal("no summary entry carries Returns; fixture must produce a returning function")
	}
	if !sawHeapTableObjects {
		t.Fatal("no summary entry carries HeapTableObjects; fixture must construct and return a table")
	}
	if !sawNormalReturnParams {
		t.Fatal("no summary entry carries NormalReturnParams; fixture must produce a normal-return parameter fact")
	}
}

// assertMutatingSlotIsIsolated corrupts slot(summary)[0] to sentinel through
// both Read and Entries, then confirms the published snapshot's slot still
// reads back its original value.
func assertMutatingSlotIsIsolated(t *testing.T, completed CompletedResult, key summary.SummaryKey, sentinel product.Value, slot func(summary.Summary) []product.Value) {
	t.Helper()
	viaRead, ok := completed.SummarySnapshot().Read(key)
	if !ok || len(slot(viaRead)) == 0 {
		t.Fatalf("Read(%v) missing target slot for mutation probe", key)
	}
	original := slot(viaRead)[0]
	slot(viaRead)[0] = sentinel

	viaEntries := entryFor(t, completed.SummarySnapshot().Entries(), key)
	slot(viaEntries)[0] = sentinel

	again, ok := completed.SummarySnapshot().Read(key)
	if !ok || slot(again)[0] != original {
		t.Fatal("mutating a summary slot through Read or Entries changed the completed snapshot")
	}
}

func assertMutatingHeapTableObjectsIsIsolated(t *testing.T, completed CompletedResult, key summary.SummaryKey) {
	t.Helper()
	viaRead, ok := completed.SummarySnapshot().Read(key)
	if !ok || len(viaRead.HeapTableObjects) == 0 {
		t.Fatalf("Read(%v) missing HeapTableObjects for mutation probe", key)
	}
	originalLen := len(viaRead.HeapTableObjects)
	for id := range viaRead.HeapTableObjects {
		delete(viaRead.HeapTableObjects, id)
	}

	viaEntries := entryFor(t, completed.SummarySnapshot().Entries(), key)
	for id := range viaEntries.HeapTableObjects {
		delete(viaEntries.HeapTableObjects, id)
	}

	again, ok := completed.SummarySnapshot().Read(key)
	if !ok || len(again.HeapTableObjects) != originalLen {
		t.Fatal("mutating HeapTableObjects through Read or Entries changed the completed snapshot")
	}
}

func entryFor(t *testing.T, entries []summary.EntrySummary, key summary.SummaryKey) summary.Summary {
	t.Helper()
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Summary
		}
	}
	t.Fatalf("no summary entry for key %v", key)
	return summary.Summary{}
}

func selectorFor(tag ResultTag) ResultSelector {
	return ResultSelector{UnitID: tag.UnitID, SolveSeq: tag.SolveSeq, Profile: tag.Profile}
}

func containsBodyInputDigest(versions map[BodyID]embedding.BodyInputDigest, want embedding.BodyInputDigest) bool {
	for _, got := range versions {
		if got == want {
			return true
		}
	}
	return false
}

func mustSolve(t *testing.T, session *BatchSession, req SolveRequest) ResultTag {
	t.Helper()
	tag, err := session.EnsureSolved(context.Background(), req)
	if err != nil {
		t.Fatalf("EnsureSolved(%s): %v", req.UnitID, err)
	}
	return tag
}

func assertStableResultContent(t *testing.T, left, right ResultTag) {
	t.Helper()
	if left.UnitDigest != right.UnitDigest ||
		left.ManifestDigest != right.ManifestDigest ||
		!reflect.DeepEqual(left.SourceDigests, right.SourceDigests) ||
		!reflect.DeepEqual(left.BodyInputDigests, right.BodyInputDigests) {
		t.Fatalf("identical solves changed content digests:\nleft  %#v\nright %#v", left, right)
	}
}
