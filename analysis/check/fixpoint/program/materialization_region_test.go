package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestMaterializedAffectedSolveRegionClosesReverseDependenciesAndSCCs(t *testing.T) {
	a := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6201)))
	b := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6202)))
	c := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6203)))
	external := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6299)))
	keys := programKeys{}
	cache := &materializedSolveCache{entries: map[materializedSolveCacheKey]materializedSolveCacheEntry{}}
	add := func(owner summary.SummaryKey, deps ...summary.SummaryKey) {
		reads := make(map[summary.SummaryKey]trackedSummaryRead, len(deps))
		for _, dep := range deps {
			reads[dep] = trackedSummaryRead{}
		}
		cache.entries[materializedSolveCacheKey{prepared: &body.Static{}, owner: owner}] = materializedSolveCacheEntry{
			routing:            materializedOwnerRoutingDigest(keys, owner),
			resolution:         summaryOwnerResolutionDigest(keys, owner),
			deps:               reads,
			noDepUniverseKnown: len(reads) == 0,
		}
	}
	add(a, external, c) // a <-> c is recursive.
	add(b, a)
	add(c, a)
	expected := map[summary.SummaryKey]struct{}{a: {}, b: {}, c: {}}

	region, ok := materializedAffectedSolveRegion(cache, materializedSummaryChanges{
		Presentation: map[summary.SummaryKey]struct{}{external: {}},
	}, keys, expected)
	if !ok {
		t.Fatal("complete dependency evidence fell back to full materialization")
	}
	for _, owner := range []summary.SummaryKey{a, b, c} {
		if !region.contains(owner) {
			t.Fatalf("affected closure missing owner %v", owner)
		}
	}
}

func TestMaterializedAffectedSolveRegionFailsClosedOnIncompleteEvidence(t *testing.T) {
	owner := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6211)))
	keys := programKeys{}
	expected := map[summary.SummaryKey]struct{}{owner: {}}
	changes := materializedSummaryChanges{Presentation: map[summary.SummaryKey]struct{}{owner: {}}}

	for name, cache := range map[string]*materializedSolveCache{
		"missing owner": {entries: map[materializedSolveCacheKey]materializedSolveCacheEntry{}},
		"unknown zero-dependency universe": {entries: map[materializedSolveCacheKey]materializedSolveCacheEntry{
			{prepared: &body.Static{}, owner: owner}: {
				routing:    materializedOwnerRoutingDigest(keys, owner),
				resolution: summaryOwnerResolutionDigest(keys, owner),
			},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := materializedAffectedSolveRegion(cache, changes, keys, expected); ok {
				t.Fatal("incomplete evidence enabled regional solve reuse")
			}
		})
	}
}
