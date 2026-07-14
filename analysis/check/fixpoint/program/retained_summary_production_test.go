package program

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
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
	cleanConfig.SummaryInputs = func() []body.SummaryInput { return trackedSummaryInputs(cleanConfig.Context, reg, tracked.deps) }
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

func TestRetainedSummaryBudgetMissFallsBackToOrdinaryPublication(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, "local x = 1\nx = x + 1\nreturn f(x)")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, body.Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99713)))
	values := []product.Value{
		typevalue.FromType(reg, typ.String),
		typevalue.FromType(reg, typ.Number),
	}
	cache := NewSummarySolveCache(reg)
	cache.retainedBudgetForTest = func(*body.Static) transfer.RetainedBudget {
		return transfer.RetainedBudget{MaxOutputs: 1}
	}
	owner := newRetainedSummaryApplicationOwner(reg)
	defer owner.Release()
	build := retainedProductionConfig(reg, dep, &body.Stats{})
	for _, value := range values {
		if _, err := cache.solveRetainedAttributed(
			prepared, "budget-fallback", 14, retainedSummaryTestSnapshot(reg, value, dep), build, owner,
			nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			t.Fatalf("budget miss escaped optimization boundary: %v", err)
		}
	}
	if owner.published == nil || owner.published.retained || owner.published.result == nil {
		t.Fatal("budget miss did not publish the exact ordinary fallback result")
	}
	want := retainedCleanSolve(t, prepared, build, retainedSummaryTestSnapshot(reg, values[1], dep))
	compareResultTrees(t, reg, want, owner.published.result, "retained-budget-ordinary-fallback")
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

func TestOrdinarySummaryMaterializationHandoffSkipsDuplicateBodySolve(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, "local total = 0\nfor i = 1, 40 do total = total + i end\nreturn f(total)")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, body.Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99711)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99712)))
	const resolution = uint64(992)
	reader := retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep)

	run := newRetainedSummaryApplicationRun(reg, true, "ordinary-handoff")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	summaryCache := NewSummarySolveCache(reg)
	if _, err := summaryCache.solveRetainedAttributed(
		prepared, "ordinary-handoff", resolution, reader,
		retainedProductionConfig(reg, dep, &body.Stats{}), owner,
		nil, nil, nil, nil, nil, nil, nil,
	); err != nil {
		t.Fatalf("ordinary summary solve: %v", err)
	}
	if owner.published == nil || owner.published.retained || owner.published.result == nil {
		t.Fatal("ordinary summary solve did not publish its completed body result")
	}

	materialStats := body.Stats{}
	materialCache := newMaterializedSolveCache(reg, run)
	materialSolves := 0
	handoff, solved, err := solveMaterializedPreparedAttributed(
		materialCache, prepared, ownerKey, 17, resolution, materializedSolveEntryState{}, reader,
		retainedProductionConfig(reg, dep, &materialStats), &materialSolves, nil,
	)
	if err != nil {
		t.Fatalf("ordinary materialization handoff: %v", err)
	}
	if solved || materialSolves != 0 || materialStats.Transfer.Solver.TransferCalls != 0 {
		t.Fatalf("ordinary handoff = solved:%v bodies:%d transfers:%d, want no body solve", solved, materialSolves, materialStats.Transfer.Solver.TransferCalls)
	}
	if owner.published.result != nil {
		t.Fatal("ordinary handoff owner still owns the transferred result")
	}

	tracked := &trackingSummaryReader{reg: reg, base: reader}
	cleanConfig := retainedProductionConfig(reg, dep, &body.Stats{})(tracked)
	cleanConfig.SummaryInputs = func() []body.SummaryInput { return trackedSummaryInputs(cleanConfig.Context, reg, tracked.deps) }
	clean, err := body.SolvePrepared(prepared, cleanConfig.SolveConfig())
	if err != nil {
		t.Fatalf("clean materialization solve: %v", err)
	}
	if handoff.ResultVersion() != clean.ResultVersion() {
		t.Fatalf("ResultVersion handoff=%d clean=%d", handoff.ResultVersion(), clean.ResultVersion())
	}
	gotSummary, err := summaryprojection.FromResultContext(context.Background(), handoff)
	if err != nil {
		t.Fatalf("handoff projection: %v", err)
	}
	wantSummary, err := summaryprojection.FromResultContext(context.Background(), clean)
	if err != nil {
		t.Fatalf("clean projection: %v", err)
	}
	if !summary.EqualNormalized(reg, gotSummary, wantSummary) {
		t.Fatal("ordinary handoff summary differs from clean solve")
	}
}

func TestRetainedSummaryMaterializationHandoffSkipsCleanBodySolveWithExactParity(t *testing.T) {
	reg := standard.Registry()
	// This shape represents the expensive edge we care about: loop work followed
	// by a call whose outcome changes during the outer summary fixed point. The
	// first application stays ordinary; its invalidated successor is retained.
	stmts := parseChunk(t, "local total = 0\nfor i = 1, 60 do total = total + i end\nreturn f(total)")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := body.PrepareBoundChunk(stmts, bindings, body.Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	dep := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99703)))
	ownerKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(99704)))
	const resolution = uint64(991)

	summaryStats := body.Stats{}
	buildSummary := retainedProductionConfig(reg, dep, &summaryStats)
	summaryCache := NewSummarySolveCache(reg)
	run := newRetainedSummaryApplicationRun(reg, true, "handoff")
	defer run.Release()
	owner := run.newOwner(ownerKey)
	readers := []summary.Snapshot{
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.String), dep),
		retainedSummaryTestSnapshot(reg, typevalue.FromType(reg, typ.Number), dep),
	}
	var summarySolves, summaryTransfers int
	for index, reader := range readers {
		if _, err := summaryCache.solveRetainedAttributed(prepared, "handoff", resolution, reader, buildSummary, owner, &summarySolves, &summaryTransfers, nil, nil, nil, nil, nil); err != nil {
			t.Fatalf("summary solve: %v", err)
		}
		if index == 0 && (owner.published == nil || owner.published.retained) {
			t.Fatal("first summary application did not stay on the ordinary path")
		}
	}
	if summarySolves != 2 || summaryTransfers == 0 {
		t.Fatalf("summary work = %d solves/%d transfers, want one ordinary and one retained full solve", summarySolves, summaryTransfers)
	}
	if owner.published == nil || owner.published.result == nil || !owner.published.retained {
		t.Fatal("summary solve did not publish a retained result")
	}
	session, ok := owner.published.resource.(*body.RetainedPreparedSession)
	if !ok || !session.Retained() {
		t.Fatal("summary solve did not retain a live generation")
	}

	materialStats := body.Stats{}
	buildMaterial := retainedProductionConfig(reg, dep, &materialStats)
	materialCache := newMaterializedSolveCache(reg, run)
	materialSolves := 0
	handoff, solved, err := solveMaterializedPreparedAttributed(
		materialCache, prepared, ownerKey, 17, resolution, materializedSolveEntryState{}, readers[1],
		buildMaterial, &materialSolves, nil,
	)
	if err != nil {
		t.Fatalf("materialization handoff: %v", err)
	}
	if solved || materialSolves != 0 || materialStats.Transfer.Solver.TransferCalls != 0 {
		t.Fatalf("handoff materialization = solved:%v bodies:%d transfers:%d, want no body solve", solved, materialSolves, materialStats.Transfer.Solver.TransferCalls)
	}
	if session.Retained() {
		t.Fatal("handoff kept the retained equation graph alive")
	}
	if owner.published.result != nil {
		t.Fatal("handoff owner still owns the transferred result")
	}

	cleanStats := body.Stats{}
	cleanBuild := retainedProductionConfig(reg, dep, &cleanStats)
	tracked := &trackingSummaryReader{reg: reg, base: readers[1]}
	cleanConfig := cleanBuild(tracked)
	cleanConfig.SummaryInputs = func() []body.SummaryInput { return trackedSummaryInputs(cleanConfig.Context, reg, tracked.deps) }
	clean, err := body.SolvePrepared(prepared, cleanConfig.SolveConfig())
	if err != nil {
		t.Fatalf("clean materialization solve: %v", err)
	}
	if cleanStats.Transfer.Solver.TransferCalls == 0 {
		t.Fatal("clean baseline unexpectedly performed no transfers")
	}
	t.Logf("materialization point transfers: handoff=0 clean=%d", cleanStats.Transfer.Solver.TransferCalls)
	if handoff.ResultVersion() != clean.ResultVersion() {
		t.Fatalf("ResultVersion handoff=%d clean=%d", handoff.ResultVersion(), clean.ResultVersion())
	}
	domain := state.Domain(reg)
	for _, point := range clean.Graph().RPO() {
		got, gotOK := handoff.StateAt(point)
		want, wantOK := clean.StateAt(point)
		if gotOK != wantOK || (gotOK && !domain.Equal(got, want)) {
			t.Fatalf("point %d state differs after handoff", point)
		}
		gotBoundary, gotBoundaryOK := handoff.StateAtBoundary(point)
		wantBoundary, wantBoundaryOK := clean.StateAtBoundary(point)
		if gotBoundaryOK != wantBoundaryOK || (gotBoundaryOK && !domain.Equal(gotBoundary, wantBoundary)) {
			t.Fatalf("point %d boundary observation differs after handoff", point)
		}
	}
	gotSummary, err := summaryprojection.FromResultContext(context.Background(), handoff)
	if err != nil {
		t.Fatalf("handoff projection: %v", err)
	}
	wantSummary, err := summaryprojection.FromResultContext(context.Background(), clean)
	if err != nil {
		t.Fatalf("clean projection: %v", err)
	}
	if !summary.EqualNormalized(reg, gotSummary, wantSummary) {
		t.Fatal("handoff materialization summary differs from clean solve")
	}
	// Rebinding deliberately recomputes the seal rather than claiming captured
	// observations from the prior provider closure are still validated. The
	// strategy counters differ, while the planned/projected observation surface
	// and every boundary state above remain exact.
	gotObservation, wantObservation := handoff.ObservationStats(), clean.ObservationStats()
	if gotObservation.PlannedNodeOutputs != wantObservation.PlannedNodeOutputs ||
		gotObservation.PlannedBoundaryOutputs != wantObservation.PlannedBoundaryOutputs ||
		gotObservation.PlannedEdgeReachability != wantObservation.PlannedEdgeReachability ||
		gotObservation.ProjectedBoundaryOutputs != wantObservation.ProjectedBoundaryOutputs ||
		gotObservation.ProjectedEdgeReachability != wantObservation.ProjectedEdgeReachability {
		t.Fatalf("observation surface handoff=%+v clean=%+v", gotObservation, wantObservation)
	}
}
