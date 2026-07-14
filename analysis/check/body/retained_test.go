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

func TestSolvePreparedRetainedFreezesStructuralCallbacksOnce(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `local value = 1; return value`), Config{
		Registry: reg, Schedule: transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatal(err)
	}
	initialCalls := make(map[cfg.Point]int)
	widenAtCalls := make(map[cfg.Point]int)
	widenDelayCalls := make(map[cfg.Point]int)
	result, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Schedule: transfer.ScheduleWTO,
		Initial: func(point cfg.Point) (state.State, bool) {
			initialCalls[point]++
			// A second observation would return the opposite presence and alter
			// the equation system. Freeze must prevent that observation.
			return state.State{}, initialCalls[point]%2 == 1 && point == prepared.cfg.Graph.Entry()
		},
		WidenAt: func(point cfg.Point) bool {
			widenAtCalls[point]++
			return widenAtCalls[point]%2 == 1
		},
		WidenDelay: func(point cfg.Point) int {
			widenDelayCalls[point]++
			return widenDelayCalls[point] % 2
		},
	}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || retained == nil {
		t.Fatal("retained solve did not publish result and session")
	}
	defer retained.Release()
	assertStructuralCallbackCallsOnce(t, prepared, "initial", initialCalls)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-at", widenAtCalls)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-delay", widenDelayCalls)
}

func TestRetainedBeginUpdateFreezesStructuralCallbacksOnce(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `local value = 1; return value`), Config{
		Registry: reg, Schedule: transfer.ScheduleWTO,
	})
	if err != nil {
		t.Fatal(err)
	}
	configFor := func(initialCalls, widenAtCalls, widenDelayCalls map[cfg.Point]int) SolveConfig {
		return SolveConfig{
			Schedule: transfer.ScheduleWTO,
			Initial: func(point cfg.Point) (state.State, bool) {
				initialCalls[point]++
				return state.State{}, point == prepared.cfg.Graph.Entry()
			},
			WidenAt: func(point cfg.Point) bool {
				widenAtCalls[point]++
				return false
			},
			WidenDelay: func(point cfg.Point) int {
				widenDelayCalls[point]++
				return 0
			},
		}
	}
	firstInitial, firstAt, firstDelay := map[cfg.Point]int{}, map[cfg.Point]int{}, map[cfg.Point]int{}
	_, retained, err := SolvePreparedRetained(prepared, configFor(firstInitial, firstAt, firstDelay), transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	assertStructuralCallbackCallsOnce(t, prepared, "initial-first", firstInitial)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-at-first", firstAt)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-delay-first", firstDelay)

	secondInitial, secondAt, secondDelay := map[cfg.Point]int{}, map[cfg.Point]int{}, map[cfg.Point]int{}
	pending, err := retained.BeginUpdate(prepared, configFor(secondInitial, secondAt, secondDelay), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Result() == nil {
		t.Fatal("retained update did not produce candidate")
	}
	if err := pending.Commit(); err != nil {
		t.Fatal(err)
	}
	if !retained.Retained() {
		t.Fatal("equal frozen inputs fell back and released retention")
	}
	assertStructuralCallbackCallsOnce(t, prepared, "initial-update", secondInitial)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-at-update", secondAt)
	assertStructuralCallbackCallsOnce(t, prepared, "widen-delay-update", secondDelay)
}

func TestRetainedStructuralCallbackCancellationPublishesNothing(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `return 1`), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(context.CancelFunc) SolveConfig{
		"initial": func(cancel context.CancelFunc) SolveConfig {
			return SolveConfig{Initial: func(cfg.Point) (state.State, bool) {
				cancel()
				return state.State{}, false
			}}
		},
		"widen-at": func(cancel context.CancelFunc) SolveConfig {
			return SolveConfig{WidenAt: func(cfg.Point) bool {
				cancel()
				return false
			}}
		},
		"widen-delay": func(cancel context.CancelFunc) SolveConfig {
			return SolveConfig{WidenDelay: func(cfg.Point) int {
				cancel()
				return 0
			}}
		},
	}
	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			config := build(cancel)
			config.Context = ctx
			config.Schedule = transfer.ScheduleWTO
			result, retained, err := SolvePreparedRetained(prepared, config, transfer.RetainedBudget{})
			if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) || result != nil || retained != nil {
				t.Fatalf("result=%v retained=%v err=%v", result != nil, retained != nil, err)
			}
		})
	}
}

func TestRetainedPreCanceledContextInvokesNoStructuralCallbacks(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `return 1`), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := 0
	result, retained, err := SolvePreparedRetained(prepared, SolveConfig{
		Context: ctx, Schedule: transfer.ScheduleWTO,
		Initial: func(cfg.Point) (state.State, bool) {
			called++
			return state.State{}, false
		},
		WidenAt: func(cfg.Point) bool {
			called++
			return false
		},
		WidenDelay: func(cfg.Point) int {
			called++
			return 0
		},
	}, transfer.RetainedBudget{})
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) || result != nil || retained != nil {
		t.Fatalf("result=%v retained=%v err=%v", result != nil, retained != nil, err)
	}
	if called != 0 {
		t.Fatalf("pre-canceled freeze invoked %d structural callbacks", called)
	}
}

func TestRetainedBeginUpdateFreezeCancellationKeepsPriorGeneration(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `return 1`), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{Schedule: transfer.ScheduleWTO}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	ctx, cancel := context.WithCancel(context.Background())
	pending, err := retained.BeginUpdate(prepared, SolveConfig{
		Context: ctx, Schedule: transfer.ScheduleWTO,
		WidenAt: func(cfg.Point) bool {
			cancel()
			return false
		},
	}, nil, true)
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) || pending != nil {
		t.Fatalf("pending=%v err=%v", pending != nil, err)
	}
	if !retained.Retained() {
		t.Fatal("freeze cancellation released prior retained generation")
	}
}

func TestRetainedRejectsLegacyResumeComposition(t *testing.T) {
	reg, _ := testRegistry(t)
	prepared, err := PrepareChunk(parseChunk(t, `return 1`), Config{Registry: reg, Schedule: transfer.ScheduleWTO})
	if err != nil {
		t.Fatal(err)
	}
	for name, config := range map[string]SolveConfig{
		"session":       {Resume: transfer.NewSession()},
		"resume-points": {ResumePoints: []cfg.Point{prepared.cfg.Graph.Entry()}},
		"empty-points":  {ResumePoints: []cfg.Point{}},
	} {
		t.Run(name, func(t *testing.T) {
			result, retained, err := SolvePreparedRetained(prepared, config, transfer.RetainedBudget{})
			if !errors.Is(err, ErrRetainedResume) || result != nil || retained != nil {
				t.Fatalf("result=%v retained=%v err=%v", result != nil, retained != nil, err)
			}
		})
	}

	_, retained, err := SolvePreparedRetained(prepared, SolveConfig{Schedule: transfer.ScheduleWTO}, transfer.RetainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	defer retained.Release()
	pending, err := retained.BeginUpdate(prepared, SolveConfig{Resume: transfer.NewSession()}, nil, true)
	if !errors.Is(err, ErrRetainedResume) || pending != nil {
		t.Fatalf("pending=%v err=%v", pending != nil, err)
	}
	if !retained.Retained() {
		t.Fatal("rejected legacy resume released retained generation")
	}
}

func assertStructuralCallbackCallsOnce(t *testing.T, prepared *Static, label string, calls map[cfg.Point]int) {
	t.Helper()
	for _, point := range prepared.cfg.Graph.RPO() {
		if calls[point] != 1 {
			t.Fatalf("%s callback calls at point %d = %d, want 1", label, point, calls[point])
		}
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
