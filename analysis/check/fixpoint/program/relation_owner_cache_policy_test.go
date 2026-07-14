package program

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestInactiveRelationOwnerCachePolicyFencesActiveConsumers(t *testing.T) {
	reg := standard.Registry()
	prepared := prepareRelationOwnerCacheTestBody(t, reg, body.Config{})
	activeKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88001)))
	inactiveKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88002)))
	policy, active, inactive := relationOwnerCacheTestPolicy(prepared, activeKey, inactiveKey)
	cache := NewSummarySolveCache(reg)
	reader := summary.NewSnapshot(reg)
	build := func(summary.Reader) body.Config { return body.Config{Registry: reg} }
	stats := &Stats{}

	if _, err := solveSummaryPrepared(cache, nil, "typed", 17, prepared, reader, build, stats, nil); err != nil {
		t.Fatalf("prewarm summary cache: %v", err)
	}
	if stats.SummaryBodySolves != 1 || stats.SummaryCacheMisses != 1 || len(cache.entries) != 1 {
		t.Fatalf("prewarm stats = solves:%d misses:%d entries:%d, want 1/1/1", stats.SummaryBodySolves, stats.SummaryCacheMisses, len(cache.entries))
	}

	activeCache := policy.summaryCache(active, cache)
	if activeCache != nil {
		t.Fatal("active consumer retained the legacy summary cache")
	}
	if _, err := solveSummaryPrepared(activeCache, nil, "typed", policy.resolutionDigest(active, 17), prepared, reader, build, stats, nil); err != nil {
		t.Fatalf("active uncached solve: %v", err)
	}
	if stats.SummaryBodySolves != 2 || stats.SummaryCacheHits != 0 || stats.SummaryCacheMisses != 1 || len(cache.entries) != 1 {
		t.Fatalf("active stats = solves:%d hits:%d misses:%d entries:%d, want 2/0/1/1", stats.SummaryBodySolves, stats.SummaryCacheHits, stats.SummaryCacheMisses, len(cache.entries))
	}

	inactiveCache := policy.summaryCache(inactive, cache)
	if inactiveCache != cache {
		t.Fatal("inactive consumer lost the configured summary cache")
	}
	if _, err := solveSummaryPrepared(inactiveCache, nil, "typed", policy.resolutionDigest(inactive, 17), prepared, reader, build, stats, nil); err != nil {
		t.Fatalf("inactive cached solve: %v", err)
	}
	if stats.SummaryBodySolves != 2 || stats.SummaryCacheHits != 1 || stats.SummaryCacheMisses != 1 || len(cache.entries) != 1 {
		t.Fatalf("inactive stats = solves:%d hits:%d misses:%d entries:%d, want 2/1/1/1", stats.SummaryBodySolves, stats.SummaryCacheHits, stats.SummaryCacheMisses, len(cache.entries))
	}

	activeDigest := policy.resolutionDigest(active, 17)
	if relationActiveV1ResolutionMarker == 0 || activeDigest == 0 || activeDigest == 17 || activeDigest != composeRelationActiveResolutionDigest(17) {
		t.Fatalf("active resolution marker/digest = marker:%x digest:%x", relationActiveV1ResolutionMarker, activeDigest)
	}
	if again := composeRelationActiveResolutionDigest(17); again != activeDigest {
		t.Fatalf("active resolution digest is nondeterministic: first=%x second=%x", activeDigest, again)
	}
	if other := composeRelationActiveResolutionDigest(18); other == activeDigest || other == 18 {
		t.Fatalf("active resolution digest collision: base17=%x base18=%x", activeDigest, other)
	}
	if got := policy.resolutionDigest(inactive, 17); got != 17 {
		t.Fatalf("inactive resolution digest = %x, want legacy 11", got)
	}
}

func TestInactiveRelationOwnerCachePolicyFencesMaterializationAndHandoff(t *testing.T) {
	reg := standard.Registry()
	prepared := prepareRelationOwnerCacheTestBody(t, reg, body.Config{Schedule: transfer.ScheduleWTO})
	activeKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88003)))
	inactiveKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88004)))
	policy, active, _ := relationOwnerCacheTestPolicy(prepared, activeKey, inactiveKey)
	legacyResolution := uint64(29)
	activeResolution := policy.resolutionDigest(active, legacyResolution)
	reader := summary.NewSnapshot(reg)
	build := func(summary.Reader) body.Config {
		return body.Config{Registry: reg, Schedule: transfer.ScheduleWTO}
	}

	materialized := newMaterializedSolveCache(reg)
	var materialSolves int
	first, solved, err := solveMaterializedPreparedAttributed(
		materialized, prepared, activeKey, 7, legacyResolution,
		materializedSolveEntryState{}, reader, infallibleMaterializedBuild(build), &materialSolves, nil,
	)
	if err != nil || !solved || materialSolves != 1 {
		t.Fatalf("legacy materialization = solved:%v solves:%d err:%v, want true/1/nil", solved, materialSolves, err)
	}
	second, solved, err := solveMaterializedPreparedAttributed(
		materialized, prepared, activeKey, 7, activeResolution,
		materializedSolveEntryState{}, reader, infallibleMaterializedBuild(build), &materialSolves, nil,
	)
	if err != nil || !solved || materialSolves != 2 || second == first {
		t.Fatalf("active materialization = solved:%v solves:%d same:%v err:%v, want true/2/false/nil", solved, materialSolves, second == first, err)
	}

	run := newRetainedSummaryApplicationRun(reg, true, "typed")
	defer run.Release()
	legacyOwner := run.newOwner(activeKey)
	summaryCache := NewSummarySolveCache(reg)
	if _, err := summaryCache.solveRetainedAttributed(
		prepared, "typed", legacyResolution, reader, build, legacyOwner,
		nil, nil, nil, nil, nil, nil, nil,
	); err != nil {
		t.Fatalf("legacy retained publication: %v", err)
	}
	if legacyOwner.published == nil || legacyOwner.published.result == nil {
		t.Fatal("legacy retained solve did not publish a handoff result")
	}
	legacyResult := legacyOwner.published.result
	handoffCache := newMaterializedSolveCache(reg, run)
	handoffSolves := 0
	_, solved, err = solveMaterializedPreparedAttributed(
		handoffCache, prepared, activeKey, 7, activeResolution,
		materializedSolveEntryState{}, reader, infallibleMaterializedBuild(build), &handoffSolves, nil,
	)
	if err != nil || !solved || handoffSolves != 1 {
		t.Fatalf("active handoff fence = solved:%v solves:%d err:%v, want true/1/nil", solved, handoffSolves, err)
	}
	if legacyOwner.published.result != legacyResult {
		t.Fatal("active resolution consumed the legacy-resolution handoff")
	}
}

func TestInactiveRelationOwnerCachePolicyCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	prepared := prepareRelationOwnerCacheTestBody(t, reg, body.Config{Schedule: transfer.ScheduleWTO})
	activeKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88005)))
	inactiveKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(88006)))
	policy, active, inactive := relationOwnerCacheTestPolicy(prepared, activeKey, inactiveKey)
	cache := NewSummarySolveCache(reg)
	run := newRetainedSummaryApplicationRun(reg, true, "typed")
	defer run.Release()
	if owner := policy.retainedOwner(active, run); owner != nil || len(run.owners) != 0 {
		t.Fatalf("active retained owner = %p, run owners = %d; want nil/0", owner, len(run.owners))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := &Stats{}
	_, err := solveSummaryPrepared(
		policy.summaryCache(active, cache), nil, "typed", policy.resolutionDigest(active, 31),
		prepared, summary.NewSnapshot(reg),
		func(summary.Reader) body.Config {
			return body.Config{Registry: reg, Schedule: transfer.ScheduleWTO, Context: ctx}
		},
		stats, nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("active canceled solve error = %v, want context.Canceled", err)
	}
	if len(cache.entries) != 0 || stats.SummaryCacheHits != 0 || stats.SummaryCacheMisses != 0 || len(run.owners) != 0 {
		t.Fatalf("canceled publication = cache:%d hits:%d misses:%d owners:%d, want 0/0/0/0", len(cache.entries), stats.SummaryCacheHits, stats.SummaryCacheMisses, len(run.owners))
	}
	if owner := policy.retainedOwner(inactive, run); owner == nil || len(run.owners) != 1 {
		t.Fatalf("inactive retained owner = %p, run owners = %d; want nonnil/1", owner, len(run.owners))
	}
}

func prepareRelationOwnerCacheTestBody(t *testing.T, reg *axis.Registry, config body.Config) *body.Static {
	t.Helper()
	if config.Registry == nil {
		config.Registry = reg
	}
	stmts := parseChunk(t, `return 1`)
	prepared, err := body.PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{}), config)
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	return prepared
}

func relationOwnerCacheTestPolicy(
	prepared *body.Static,
	activeKey, inactiveKey summary.SummaryKey,
) (relationOwnerCachePolicy, relationConsumerIdentity, relationConsumerIdentity) {
	generation := &relationCatalogGeneration{}
	active := relationConsumerIdentity{Summary: activeKey, BodyDigest: prepared.IdentityDigest(), Prepared: prepared, Generation: generation}
	inactive := relationConsumerIdentity{Summary: inactiveKey, BodyDigest: prepared.IdentityDigest(), Prepared: prepared, Generation: generation}
	consumers := relationConsumerPolicy{
		entries:    []relationConsumerEntry{{identity: active, active: true}, {identity: inactive}},
		byKey:      map[summary.SummaryKey]int{activeKey: 0, inactiveKey: 1},
		generation: generation,
	}
	return newRelationOwnerCachePolicy(consumers), active, inactive
}
