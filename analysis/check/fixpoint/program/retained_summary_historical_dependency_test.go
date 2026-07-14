package program

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRetainedSummaryHistoricalPointReadCannotBecomeNoopUpdate(t *testing.T) {
	reg := standard.Registry()
	stableKey := dependencyTestKey(281)
	historicalKey := dependencyTestKey(282)
	stable := typevalue.FromType(reg, typ.String)
	before := typevalue.FromType(reg, typ.Number)
	after := typevalue.FromType(reg, typ.Boolean)

	reader := newPointTrackingSummaryReader(reg, summary.NewSnapshot(reg,
		summary.EntrySummary{Key: stableKey, Summary: summary.Summary{Returns: []product.Value{stable}}},
		summary.EntrySummary{Key: historicalKey, Summary: summary.Summary{Returns: []product.Value{before}}},
	))
	point := cfg.Point(7)

	// The first visit observes both summaries. A later visit of the same point
	// no longer reads historicalKey, so the latest-transfer point ledger drops
	// it even though the generation-wide exact reader still records the read.
	reader.tracker.before(point)
	_, _ = reader.Read(stableKey)
	_, _ = reader.Read(historicalKey)
	reader.tracker.after(point)
	reader.tracker.before(point)
	_, _ = reader.Read(stableKey)
	reader.tracker.after(point)

	dependencies := reader.dependencies()
	if len(reader.base.deps) != 2 {
		t.Fatalf("generation dependencies = %d, want both stable and historical reads", len(reader.base.deps))
	}
	if reads := dependencies.byPoint[point]; len(reads) != 1 {
		t.Fatalf("latest point dependencies = %d, want only the stable read", len(reads))
	} else if _, ok := reads[historicalKey]; ok {
		t.Fatal("setup retained the historical read in the latest-transfer ledger")
	}

	key := retainedSummaryApplicationKey{body: 81, input: 82, profile: "historical-read", resolution: 83}
	owner := newRetainedSummaryApplicationOwner(reg)
	defer owner.Release()
	owner.published = &retainedSummaryApplicationPublication{
		key:      key,
		deps:     normalizedRetainedSummaryDependencies(reg, reader.base.deps),
		points:   dependencies,
		retained: true,
		resource: &retainedSummaryTestResource{},
	}

	changed := summary.NewSnapshot(reg,
		summary.EntrySummary{Key: stableKey, Summary: summary.Summary{Returns: []product.Value{stable}}},
		summary.EntrySummary{Key: historicalKey, Summary: summary.Summary{Returns: []product.Value{after}}},
	)
	decision := owner.begin(key, changed).Decision()
	if decision.kind != retainedSummaryApplyRegional || len(decision.changed) != 1 || decision.changed[0] != historicalKey {
		t.Fatalf("decision = %+v, want one regional historical dependency revision", decision)
	}
	if len(decision.points) != 0 || !decision.forceFull || decision.reproject {
		t.Fatalf("decision = %+v: a changed generation dependency with no latest owner must force a full replay, never produce a no-op regional update", decision)
	}
}

func TestRetainedSummaryChangedDependencyOrderIsDeterministic(t *testing.T) {
	reg := standard.Registry()
	low, high := dependencyTestKey(283), dependencyTestKey(284)
	before := typevalue.FromType(reg, typ.Number)
	after := typevalue.FromType(reg, typ.Boolean)
	owner := newRetainedSummaryApplicationOwner(reg)
	defer owner.Release()
	key := retainedSummaryApplicationKey{body: 84}
	owner.published = &retainedSummaryApplicationPublication{
		key: key,
		deps: map[summary.SummaryKey]trackedSummaryRead{
			high: retainedSummaryTestRead(before),
			low:  retainedSummaryTestRead(before),
		},
		points: pointSummaryDependencies{byPoint: map[cfg.Point]map[summary.SummaryKey]pointSummaryRead{
			8: {high: {present: true, digest: 1}},
			3: {low: {present: true, digest: 1}},
		}},
		retained: true,
		resource: &retainedSummaryTestResource{},
	}
	reader := summary.NewSnapshot(reg,
		summary.EntrySummary{Key: high, Summary: summary.Summary{Returns: []product.Value{after}}},
		summary.EntrySummary{Key: low, Summary: summary.Summary{Returns: []product.Value{after}}},
	)

	decision := owner.begin(key, reader).Decision()
	if !slices.Equal(decision.changed, []summary.SummaryKey{low, high}) {
		t.Fatalf("changed dependency order = %v, want [%v %v]", decision.changed, low, high)
	}
	if !slices.Equal(decision.points, []cfg.Point{3, 8}) || decision.forceFull || decision.reproject {
		t.Fatalf("deterministic owned decision = %+v, want points [3 8] without fallback", decision)
	}
}

func TestRetainedSummaryHistoricalFallbackAbortPreservesPublication(t *testing.T) {
	reg := standard.Registry()
	dependency := dependencyTestKey(285)
	before := typevalue.FromType(reg, typ.Number)
	after := typevalue.FromType(reg, typ.Boolean)
	key := retainedSummaryApplicationKey{body: 85}
	resource := &retainedSummaryTestResource{}
	owner := newRetainedSummaryApplicationOwner(reg)
	defer owner.Release()
	published := &retainedSummaryApplicationPublication{
		key:      key,
		deps:     map[summary.SummaryKey]trackedSummaryRead{dependency: retainedSummaryTestRead(before)},
		retained: true,
		resource: resource,
	}
	owner.published = published
	changed := retainedSummaryTestSnapshot(reg, after, dependency)

	attempt := owner.begin(key, changed)
	decision := attempt.Decision()
	if decision.kind != retainedSummaryApplyRegional || !decision.forceFull || len(decision.points) != 0 {
		t.Fatalf("historical fallback decision = %+v, want full regional update", decision)
	}
	attempt.Abort()
	if owner.published != published || resource.releases != 0 {
		t.Fatalf("aborted fallback changed publication/resource = %p/%d, want %p/0", owner.published, resource.releases, published)
	}
	retry := owner.begin(key, changed).Decision()
	if retry.kind != retainedSummaryApplyRegional || !retry.forceFull || len(retry.points) != 0 {
		t.Fatalf("retry after abort = %+v, want same full regional update", retry)
	}
}
