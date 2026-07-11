package program

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRetainedSummaryProductionRegionalRevisionMatchesCleanWithLessWork(t *testing.T) {
	reg := standard.Registry()
	source := strings.Repeat("local x = 1\nx = x + 1\n", 40) + "return f()"
	stmts := parseChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, body.Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99701)))
	bodyStats := body.Stats{}
	build := retainedProductionConfig(reg, dep, &bodyStats)
	cache := NewSummarySolveCache(reg)
	owner := newRetainedSummaryApplicationOwner(reg)
	defer owner.Release()

	values := []product.Value{
		typevalue.FromType(reg, typ.String),
		typevalue.FromType(reg, typ.Number),
		typevalue.FromType(reg, typ.Boolean),
	}
	var solves, transfers, changes, changedTransfers, hits, misses int
	var summaries []summary.Summary
	deltas := make([]int, 0, len(values))
	for _, value := range values {
		before := transfers
		got, err := cache.solveRetainedAttributed(
			prepared, "typed", 1, retainedSummaryTestSnapshot(reg, value, dep), build, owner,
			&solves, &transfers, &changes, &changedTransfers, &hits, &misses, nil,
		)
		if err != nil {
			t.Fatalf("solveRetainedAttributed: %v", err)
		}
		summaries = append(summaries, got)
		deltas = append(deltas, transfers-before)
	}
	if owner.published == nil || !owner.published.retained || owner.published.result == nil {
		t.Fatal("third revision did not retain a published body generation")
	}
	if deltas[2] >= deltas[1] {
		t.Fatalf("regional transfers = %d, clean retained build = %d; want regional less", deltas[2], deltas[1])
	}
	t.Logf("point transfers: ordinary=%d retained-build=%d regional=%d", deltas[0], deltas[1], deltas[2])
	if changes != 2 || changedTransfers != deltas[1]+deltas[2] {
		t.Fatalf("dependency revision stats = changes:%d transfers:%d, want 2/%d", changes, changedTransfers, deltas[1]+deltas[2])
	}

	cleanReader := retainedSummaryTestSnapshot(reg, values[2], dep)
	tracked := &trackingSummaryReader{reg: reg, base: cleanReader}
	cleanConfig := build(tracked)
	cleanConfig.SummaryInputDigests = func() []uint64 { return trackedSummaryReadDigests(reg, tracked.deps) }
	clean, err := body.SolvePrepared(prepared, cleanConfig.SolveConfig())
	if err != nil {
		t.Fatalf("clean SolvePrepared: %v", err)
	}
	if owner.published.result.ResultVersion() != clean.ResultVersion() {
		t.Fatalf("regional ResultVersion = %d, clean = %d", owner.published.result.ResultVersion(), clean.ResultVersion())
	}
	cleanSummary, _, err := projectRetainedSummary(cleanConfig, prepared, clean, &pointSummaryDependencyTracker{reg: reg}, false)
	if err != nil {
		t.Fatalf("clean projection: %v", err)
	}
	if !summary.EqualNormalized(reg, summaries[2], cleanSummary) {
		t.Fatal("regional projected summary differs from clean solve")
	}
}

func retainedProductionConfig(reg *axis.Registry, dep summary.SummaryKey, stats *body.Stats) func(summary.Reader) body.Config {
	return func(reader summary.Reader) body.Config {
		return body.Config{
			Registry: reg, Stats: stats, Schedule: transfer.ScheduleWTO,
			CallOutcome: func(_ transfer.NodeContext, _ factflow.CallSiteView, _ state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
				sum, ok := reader.Read(dep)
				if !ok || len(sum.Returns) == 0 {
					return callpayload.CallOutcome{}
				}
				return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: sum.Returns[0]}}}
			},
		}
	}
}

func TestRetainedSummaryProductionCancellationPublishesNothingAndReleases(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, "local x = 1\nreturn f()")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, body.Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99702)))
	baseBuild := retainedProductionConfig(reg, dep, &body.Stats{})
	ctx := context.Background()
	build := func(reader summary.Reader) body.Config {
		config := baseBuild(reader)
		config.Context = ctx
		return config
	}
	cache := NewSummarySolveCache(reg)
	owner := newRetainedSummaryApplicationOwner(reg)
	values := []product.Value{
		typevalue.FromType(reg, typ.String),
		typevalue.FromType(reg, typ.Number),
		typevalue.FromType(reg, typ.Boolean),
	}
	for _, value := range values[:2] {
		if _, err := cache.solveRetainedAttributed(prepared, "typed", 2, retainedSummaryTestSnapshot(reg, value, dep), build, owner, nil, nil, nil, nil, nil, nil, nil); err != nil {
			t.Fatalf("setup solve: %v", err)
		}
	}
	published := owner.published
	session, ok := published.resource.(*body.RetainedPreparedSession)
	if !ok || !session.Retained() {
		t.Fatal("setup did not retain a live generation")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx = canceled
	if _, err := cache.solveRetainedAttributed(prepared, "typed", 2, retainedSummaryTestSnapshot(reg, values[2], dep), build, owner, nil, nil, nil, nil, nil, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled update error = %v, want context.Canceled", err)
	}
	if owner.published != published || !session.Retained() {
		t.Fatal("canceled update advanced or released the published generation")
	}

	ctx = context.Background()
	if _, err := cache.solveRetainedAttributed(prepared, "typed", 2, retainedSummaryTestSnapshot(reg, values[2], dep), build, owner, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	owner.Release()
	if session.Retained() {
		t.Fatal("owner Release retained the prior generation")
	}
}
