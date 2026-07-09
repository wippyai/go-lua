package service

import (
	"context"
	"testing"
)

func TestEnsureSolvedResolvesCurrentDocumentVersionBeforeCacheReuse(t *testing.T) {
	ctx := context.Background()
	session := NewBatchSession()
	input := UnitInput{
		ID:              "document-version-cache",
		ModulePath:      "example/document-version-cache",
		EntryFile:       "main.lua",
		DocumentVersion: 1,
		SourceFiles: map[string][]byte{
			"main.lua": []byte("local value: number = 1\nreturn value\n"),
		},
	}
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("initial UpsertUnit: %v", err)
	}
	first, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID})
	if err != nil {
		t.Fatalf("initial EnsureSolved: %v", err)
	}

	input.DocumentVersion = 2
	state, err := session.UpsertUnit(ctx, input)
	if err != nil {
		t.Fatalf("document-only UpsertUnit: %v", err)
	}
	if state.Changed {
		t.Fatal("document-only upsert unexpectedly changed semantic input")
	}
	stale, err := session.PlacementPlan(ctx, PlacementPlanRequest{Selector: ResultSelector{UnitID: input.ID}})
	if err != nil {
		t.Fatalf("stale PlacementPlan: %v", err)
	}
	if !stale.Meta.Stale {
		t.Fatal("older document result was reported current before re-solve")
	}
	second, err := session.EnsureSolved(ctx, SolveRequest{UnitID: input.ID})
	if err != nil {
		t.Fatalf("document-only EnsureSolved: %v", err)
	}
	if second.SolveSeq <= first.SolveSeq {
		t.Fatalf("document-only solve reused sequence %d, want newer than %d", second.SolveSeq, first.SolveSeq)
	}
	if second.DocumentVersion != input.DocumentVersion {
		t.Fatalf("resolved document version = %d, want %d", second.DocumentVersion, input.DocumentVersion)
	}

	result, err := session.PlacementPlan(ctx, PlacementPlanRequest{Selector: ResultSelector{UnitID: input.ID}})
	if err != nil {
		t.Fatalf("PlacementPlan: %v", err)
	}
	if result.Meta.Stale {
		t.Fatal("current document result was marked stale")
	}
	if result.Meta.Tag.DocumentVersion != input.DocumentVersion {
		t.Fatalf("query document version = %d, want %d", result.Meta.Tag.DocumentVersion, input.DocumentVersion)
	}
}
