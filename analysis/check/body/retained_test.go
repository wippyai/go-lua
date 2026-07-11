package body

import (
	"context"
	"errors"
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestRetainedPreparedUpdateMatchesCleanSolveAcrossSummaryChange(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f(); return out`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	firstConfig := SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markLow)),
		SummaryInputDigests: func() []uint64 { return []uint64{1} },
	}
	_, retained, err := SolvePreparedRetained(prepared, firstConfig, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()

	secondConfig := SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markHigh)),
		SummaryInputDigests: func() []uint64 { return []uint64{2} },
	}
	changed := []cfg.Point{retainedCallPoint(t, prepared)}
	pending, err := retained.BeginUpdate(prepared, secondConfig, changed, false)
	if err != nil {
		t.Fatal(err)
	}
	got := pending.Result()
	if err := pending.Commit(); err != nil {
		t.Fatal(err)
	}
	want, err := SolvePrepared(prepared, secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedResultEqual(t, got, want)
}

func TestRetainedPreparedCanceledAfterConvergenceDoesNotAdvanceSession(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f(); return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}}), Config{
		Registry: reg, Schedule: transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markLow)),
	}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()

	ctx, cancel := context.WithCancel(context.Background())
	canceledConfig := SolveConfig{
		Context: ctx, Schedule: transfer.ScheduleWTO,
		CallOutcome: staticCallOutcome(markedValue(reg, markKey, markHigh)),
		// ResultVersion runs after transfer convergence and observation sealing.
		SummaryInputDigests: func() []uint64 { cancel(); return []uint64{2} },
	}
	changed := []cfg.Point{retainedCallPoint(t, prepared)}
	pending, err := retained.BeginUpdate(prepared, canceledConfig, changed, false)
	var result *Result
	if pending != nil {
		result = pending.Result()
	}
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("canceled update result=%v err=%v", result != nil, err)
	}
	if !retained.Retained() {
		t.Fatal("canceled update released the prior generation")
	}

	retryConfig := SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markHigh)),
		SummaryInputDigests: func() []uint64 { return []uint64{2} },
	}
	pending, err = retained.BeginUpdate(prepared, retryConfig, changed, false)
	if err != nil {
		t.Fatal(err)
	}
	got := pending.Result()
	if err := pending.Commit(); err != nil {
		t.Fatal(err)
	}
	want, err := SolvePrepared(prepared, retryConfig)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedResultEqual(t, got, want)
}

func TestRetainedPreparedIdentityMismatchReleasesAndFallsBack(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = 1; return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{}), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{Schedule: transfer.ScheduleWTO}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	entry := state.State{}.WriteValue(reg, statekey.ReturnSlot(99), markedValue(reg, markKey, markLow))
	pending, err := retained.BeginUpdate(prepared, SolveConfig{Schedule: transfer.ScheduleWTO, EntryState: entry}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got := pending.Result()
	if err := pending.Commit(); err != nil {
		t.Fatal(err)
	}
	if retained.Retained() {
		t.Fatal("identity mismatch retained the old equation generation")
	}
	want, err := SolvePrepared(prepared, SolveConfig{Schedule: transfer.ScheduleWTO, EntryState: entry})
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedResultEqual(t, got, want)
}

func TestRetainedPreparedDownstreamRejectionAbortsUpdate(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f(); return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}}), Config{
		Registry: reg, Schedule: transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markLow)),
	}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	config := SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markHigh)),
		SummaryInputDigests: func() []uint64 { return []uint64{2} },
	}
	changed := []cfg.Point{retainedCallPoint(t, prepared)}
	rejected, err := retained.BeginUpdate(prepared, config, changed, false)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Result() == nil {
		t.Fatal("validated body result missing before downstream projection")
	}
	// Simulate summary projection rejecting/canceling the candidate.
	rejected.Abort()
	if rejected.Result() != nil {
		t.Fatal("aborted candidate still publishes a body result")
	}

	retry, err := retained.BeginUpdate(prepared, config, changed, false)
	if err != nil {
		t.Fatal(err)
	}
	got := retry.Result()
	if err := retry.Commit(); err != nil {
		t.Fatal(err)
	}
	want, err := SolvePrepared(prepared, config)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedResultEqual(t, got, want)
}

func TestRetainedPreparedPostFlowPanicAbortsUpdate(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f(); return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}}), Config{
		Registry: reg, Schedule: transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markLow)),
	}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	changed := []cfg.Point{retainedCallPoint(t, prepared)}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("post-flow ResultVersion panic was not propagated")
			}
		}()
		_, _ = retained.BeginUpdate(prepared, SolveConfig{
			Schedule:            transfer.ScheduleWTO,
			CallOutcome:         staticCallOutcome(markedValue(reg, markKey, markHigh)),
			SummaryInputDigests: func() []uint64 { panic("projection panic") },
		}, changed, false)
	}()

	config := SolveConfig{
		Schedule: transfer.ScheduleWTO, CallOutcome: staticCallOutcome(markedValue(reg, markKey, markHigh)),
		SummaryInputDigests: func() []uint64 { return []uint64{2} },
	}
	retry, err := retained.BeginUpdate(prepared, config, changed, false)
	if err != nil {
		t.Fatalf("retry after panic: %v", err)
	}
	got := retry.Result()
	if err := retry.Commit(); err != nil {
		t.Fatal(err)
	}
	want, err := SolvePrepared(prepared, config)
	if err != nil {
		t.Fatal(err)
	}
	assertRetainedResultEqual(t, got, want)
}

func TestRetainedPreparedBudgetPublishesNothing(t *testing.T) {
	reg, _ := testRegistry(t)
	stmts := parseChunk(t, `local out = 1; return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{}), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	result, retained, err := SolvePreparedRetained(prepared, SolveConfig{Schedule: transfer.ScheduleWTO}, transfer.RetainedBudget{MaxOutputs: 1})
	if !errors.Is(err, solve.ErrRetainedBudget) || result != nil || retained != nil {
		t.Fatalf("budget result=%v retained=%v err=%v", result != nil, retained != nil, err)
	}
}

func TestRetainedPreparedInitialLateCancellationPublishesNothing(t *testing.T) {
	reg, _ := testRegistry(t)
	stmts := parseChunk(t, `local out = 1; return out`)
	prepared, err := PrepareBoundChunk(stmts, bind.BindChunk(stmts, bind.Options{}), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Context: ctx, Schedule: transfer.ScheduleWTO,
		SummaryInputDigests: func() []uint64 { cancel(); return []uint64{1} },
	}, transfer.RetainedBudget{})
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) || result != nil || retained != nil {
		t.Fatalf("late cancellation result=%v retained=%v err=%v", result != nil, retained != nil, err)
	}
}

func retainedCallPoint(t *testing.T, prepared *Static) cfg.Point {
	t.Helper()
	var points []cfg.Point
	for _, point := range prepared.cfg.Graph.RPO() {
		if _, ok := prepared.facts.CallSiteView(point); ok {
			points = append(points, point)
		}
	}
	if len(points) != 1 {
		t.Fatalf("call points = %v, want one", points)
	}
	return points[0]
}

func assertRetainedResultEqual(t *testing.T, got, want *Result) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("nil result: got=%v want=%v", got == nil, want == nil)
	}
	if got.ResultVersion() != want.ResultVersion() {
		t.Fatalf("ResultVersion=%d, want %d", got.ResultVersion(), want.ResultVersion())
	}
	domain := state.Domain(got.registry)
	for _, point := range got.cfg.Graph.RPO() {
		if !domain.Equal(got.flow[point], want.flow[point]) {
			t.Fatalf("flow differs at point %d", point)
		}
		if got.published.pointReachable[point] != want.published.pointReachable[point] {
			t.Fatalf("reachability differs at point %d", point)
		}
		gotNode, gotOK := got.published.nodeOutputs[point]
		wantNode, wantOK := want.published.nodeOutputs[point]
		if gotOK != wantOK || (gotOK && !domain.Equal(gotNode, wantNode)) {
			t.Fatalf("published node differs at point %d", point)
		}
	}
}
