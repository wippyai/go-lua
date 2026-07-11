package program

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func dependencyTestKey(id symbol.ID) summary.SummaryKey {
	return summary.DefaultSummaryKey(ref.FromSymbol(id))
}

func TestPointSummaryDependenciesOwnReadsAndComposeHooks(t *testing.T) {
	key := dependencyTestKey(101)
	reg := standard.Registry()
	base := summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{}})
	reader := newPointTrackingSummaryReader(reg, base)
	tracker := reader.tracker
	beforeCalls, afterCalls := 0, 0
	config := body.Config{
		BeforePoint: func(cfg.Point) { beforeCalls++ },
		AfterPoint:  func(cfg.Point) { afterCalls++ },
	}
	attachPointSummaryTracking(&config, tracker)
	config.BeforePoint(7)
	_, _ = reader.Read(key)
	config.AfterPoint(7)

	deps := tracker.snapshot()
	points, fallback := deps.affectedPoints([]summary.SummaryKey{key})
	if !slices.Equal(points, []cfg.Point{7}) || fallback {
		t.Fatalf("affected points/fallback = %v/%v, want [7]/false", points, fallback)
	}
	if beforeCalls != 1 || afterCalls != 1 {
		t.Fatalf("composed hooks = %d/%d, want 1/1", beforeCalls, afterCalls)
	}
	tokens := deps.tokens()
	wantDigest := uint64(summary.NormalizedPayloadDigest(reg, summary.Normalize(reg, summary.Summary{})))
	if len(tokens) != 1 || !tokens[0].Owned || !tokens[0].Present || tokens[0].Digest != wantDigest {
		t.Fatalf("owned normalized token = %v, want present digest %d", tokens, wantDigest)
	}
}

func TestPointSummaryDependenciesLatestTransferDropsDisappearedRead(t *testing.T) {
	key := dependencyTestKey(102)
	reader := newPointTrackingSummaryReader(standard.Registry(), nil)
	tracker := reader.tracker
	tracker.before(9)
	_, _ = reader.Read(key)
	tracker.after(9)
	tracker.before(9)
	tracker.after(9)

	points, fallback := tracker.snapshot().affectedPoints([]summary.SummaryKey{key})
	if len(points) != 0 || fallback {
		t.Fatalf("disappeared read affected points/fallback = %v/%v, want none/false", points, fallback)
	}
}

func TestPointSummaryDependenciesUnownedChangedReadForcesFallback(t *testing.T) {
	key := dependencyTestKey(103)
	reader := newPointTrackingSummaryReader(standard.Registry(), nil)
	tracker := reader.tracker
	_, _ = reader.Read(key)

	points, fallback := tracker.snapshot().affectedPoints([]summary.SummaryKey{key})
	if len(points) != 0 || !fallback {
		t.Fatalf("unowned affected points/fallback = %v/%v, want none/true", points, fallback)
	}
	tokens := tracker.snapshot().tokens()
	if len(tokens) != 1 || tokens[0].Owned {
		t.Fatalf("unowned read token = %v, want one unowned token", tokens)
	}
}

func TestPointSummaryDependencyTokensAreDeterministicAndDetached(t *testing.T) {
	reg := standard.Registry()
	a, b := dependencyTestKey(104), dependencyTestKey(105)
	build := func(reverse bool) pointSummaryDependencies {
		reader := newPointTrackingSummaryReader(reg, nil)
		tracker := reader.tracker
		tracker.before(11)
		if reverse {
			_, _ = reader.Read(b)
			_, _ = reader.Read(a)
		} else {
			_, _ = reader.Read(a)
			_, _ = reader.Read(b)
		}
		tracker.after(11)
		return tracker.snapshot()
	}
	first := build(false)
	tokens := first.tokens()
	if got := build(true).tokens(); !slices.Equal(tokens, got) {
		t.Fatalf("tokens depend on read order: first=%v second=%v", tokens, got)
	}
	// Returned tokens contain values only. Mutating the returned slice cannot
	// mutate the published snapshot.
	tokens[0].Point = 99
	if got := first.tokens(); got[0].Point != 11 {
		t.Fatalf("token snapshot was mutable: %v", got)
	}
}

func TestSummarySolveCacheCanceledSolvePublishesNothing(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `return 1`)
	prepared, err := body.PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{}), body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache := NewSummarySolveCache(reg)
	_, err = cache.solve(prepared, "typed", 0, summary.NewSnapshot(reg), func(summary.Reader) body.Config {
		return body.Config{Registry: reg, Context: ctx}
	}, nil, nil, nil, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled solve error = %v, want context.Canceled", err)
	}
	if len(cache.entries) != 0 {
		t.Fatalf("canceled solve published %d cache entries", len(cache.entries))
	}
}

func BenchmarkPointSummaryDependencyTracking(b *testing.B) {
	reg := standard.Registry()
	key := dependencyTestKey(106)
	base := summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{}})
	b.Run("existing-exact-reader", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			reader := &trackingSummaryReader{reg: reg, base: base}
			_, _ = reader.Read(key)
		}
	})
	b.Run("opt-in-point-attribution", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			reader := newPointTrackingSummaryReader(reg, base)
			reader.tracker.before(7)
			_, _ = reader.Read(key)
			reader.tracker.after(7)
			_ = reader.dependencies()
		}
	})
}
