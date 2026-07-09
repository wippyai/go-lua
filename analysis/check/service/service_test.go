package service

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	checkdiagnostics "github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/manifest"
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
local function identity(value: number): number
	return value
end
local wrong = missing_value
local exported = { value = identity(1) }
return exported
`),
		},
		Profile: "typed",
	}
	state, err := session.UpsertUnit(ctx, input)
	if err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	if state.UnitDigest.IsZero() || state.SourceDigests["main.lua"].IsZero() || !state.Changed {
		t.Fatalf("unit state = %#v, want new content digests", state)
	}

	tag, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID, Trigger: TriggerBatch})
	if err != nil {
		t.Fatalf("EnsureSolved: %v", err)
	}
	if tag.SolveSeq == 0 || tag.UnitDigest != state.UnitDigest || tag.ManifestDigest.IsZero() {
		t.Fatalf("result tag = %#v, want complete digest tag", tag)
	}
	if len(tag.BodyVersions) < 2 || tag.BodyVersions["root"] == 0 {
		t.Fatalf("body versions = %#v, want root and nested function", tag.BodyVersions)
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
		if item.ResultVersion == 0 || !containsBodyVersion(tag.BodyVersions, item.ResultVersion) {
			t.Fatalf("judgment %s ResultVersion = %d, body versions = %#v", item.Code, item.ResultVersion, tag.BodyVersions)
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
	versions, err := session.BodyResultVersions(ctx, BodyResultVersionsRequest{Selector: selectorFor(tag)})
	if err != nil {
		t.Fatalf("BodyResultVersions: %v", err)
	}
	if !reflect.DeepEqual(versions.Versions, tag.BodyVersions) {
		t.Fatalf("body version query = %#v, want %#v", versions.Versions, tag.BodyVersions)
	}

	assertCompletedResultIsDefensivelyImmutable(t, completed)
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
	if !bState.Changed || bState.UnitDigest == b1.UnitDigest || bState.SourceDigests["b.lua"] == b1.SourceDigests["b.lua"] {
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
	if b2.UnitDigest == b1.UnitDigest || b2.SourceDigests["b.lua"] == b1.SourceDigests["b.lua"] {
		t.Fatalf("B source edit reused unit/source digest: before=%#v after=%#v", b1, b2)
	}
	if b2.ManifestDigest != b1.ManifestDigest || !reflect.DeepEqual(b2.BodyVersions, b1.BodyVersions) {
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

func assertCompletedResultIsDefensivelyImmutable(t *testing.T, completed CompletedResult) {
	t.Helper()
	tag := completed.Tag()
	originalSource := tag.SourceDigests["main.lua"]
	tag.SourceDigests["main.lua"] = Digest{}
	tag.BodyVersions["root"] = 0
	if again := completed.Tag(); again.SourceDigests["main.lua"] != originalSource || again.BodyVersions["root"] == 0 {
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
}

func selectorFor(tag ResultTag) ResultSelector {
	return ResultSelector{UnitID: tag.UnitID, ResultVersion: tag.SolveSeq, Profile: tag.Profile}
}

func containsBodyVersion(versions map[BodyID]uint64, want uint64) bool {
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
		!reflect.DeepEqual(left.BodyVersions, right.BodyVersions) {
		t.Fatalf("identical solves changed content digests:\nleft  %#v\nright %#v", left, right)
	}
}
