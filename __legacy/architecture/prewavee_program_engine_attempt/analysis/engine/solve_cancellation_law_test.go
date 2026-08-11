package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TestSolveCancellationAbandonsTheWholeGeneration uses only the caller's
// context and public Rule/Query APIs. A canceled generation has neither a
// State nor a reusable partial result; the same sealed Solver can immediately
// evaluate a fresh caller request from its original immutable baseline.
func TestSolveCancellationAbandonsTheWholeGeneration(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 81)
	query := localLawQuery(t, solver, factor, shard, entry)
	localLawDeclareAt(t, solver, factor, 82, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawBoth)
	})
	if !solver.Seal() {
		t.Fatal("Seal rejected cancellation law")
	}

	if state, complete := solver.Solve(nil, nil); complete || state != nil {
		t.Fatal("nil caller context published a State")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if state, complete := solver.Solve(canceled, nil); complete || state != nil {
		t.Fatal("canceled caller context published a State")
	}

	state, complete := solver.Solve(context.Background(), nil)
	if !complete || state == nil {
		t.Fatal("fresh caller request did not complete after cancellation")
	}
	if got := localLawRead(t, query, state); got != localLawBoth {
		t.Fatalf("fresh State = %d, want %d", got, localLawBoth)
	}

	canceled, cancel = context.WithCancel(context.Background())
	cancel()
	if reused, complete := solver.Solve(canceled, state); complete || reused != nil {
		t.Fatal("canceled caller reused a completed State")
	}
	if reused, complete := solver.Solve(context.Background(), state); !complete || reused != state {
		t.Fatal("unchanged State was not reused after canceled caller")
	}
}

// TestSolveCancellationDuringRuleExecutionRollsBack proves the same terminal
// cut when cancellation arrives after an action has started. The callback is
// an ordinary public domain Rule; it sees no transaction or publication
// authority. Its first request cancels externally, publishes nothing, and the
// next request still evaluates the exact Rule semantics.
func TestSolveCancellationDuringRuleExecutionRollsBack(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 83)
	query := localLawQuery(t, solver, factor, shard, entry)

	caller, cancel := context.WithCancel(context.Background())
	localLawDeclareAt(t, solver, factor, 84, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		cancel()
		return access.Set(0, localLawOne)
	})
	if !solver.Seal() {
		t.Fatal("Seal rejected in-flight cancellation law")
	}

	if state, complete := solver.Solve(caller, nil); complete || state != nil {
		t.Fatal("in-flight cancellation published a partial State")
	}
	state, complete := solver.Solve(context.Background(), nil)
	if !complete || state == nil {
		t.Fatal("fresh request did not complete after in-flight cancellation")
	}
	if got := localLawRead(t, query, state); got != localLawOne {
		t.Fatalf("fresh State = %d, want %d", got, localLawOne)
	}
}

// TestSolveCompletedStateReuseIsAllocationFree is the steady-state law for
// the normal caller path. The completed State is immutable, so a valid
// background request returns that sole State without opening an epoch.
func TestSolveCompletedStateReuseIsAllocationFree(t *testing.T) {
	solver, value, shard := localLawSolver(t, "local kept = 1")
	entry := localLawEntry(t, value)
	factor := localLawFactor(t, solver, 85)
	localLawDeclareAt(t, solver, factor, 86, shard, entry, func(access engine.Access[uint64, localLawBits]) bool {
		return access.Set(0, localLawOne)
	})
	query := localLawQuery(t, solver, factor, shard, entry)
	state := localLawSealAndSolve(t, solver)
	if got := localLawRead(t, query, state); got != localLawOne {
		t.Fatalf("initial State = %d, want %d", got, localLawOne)
	}

	background := context.Background()
	allocations := testing.AllocsPerRun(100, func() {
		reused, complete := solver.Solve(background, state)
		if !complete || reused != state {
			t.Fatal("unchanged solve did not reuse its completed State")
		}
	})
	if allocations != 0 {
		t.Fatalf("unchanged completed solve allocations = %g, want 0", allocations)
	}
}
