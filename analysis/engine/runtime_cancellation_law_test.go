package engine

import (
	"context"
	"testing"
)

// TestCancellationDuringProductDropsStagedPatchBeforePublication cancels at
// the only point where a Product callback has already staged a typed output.
// The callback span is necessarily synchronous, but the immediately following
// Product checkpoint must reject the frame and the deferred output cut must
// discard its unpublished patch. A fresh epoch then proves no partial result
// escaped into reusable runtime state.
func TestCancellationDuringProductDropsStagedPatchBeforePublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := 0
	solver, query, receipt, assembled := recurrenceSolverFixtureWithCallbacks(t, true, nil, func(access Access[uint64, ruleUnit]) bool {
		called++
		if called > 1 {
			return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
		}
		return Product(access, func(row Row) bool {
			if !StageValue(access, row, 1) {
				return false
			}
			cancel()
			return true
		})
	}, nil)
	if !assembled || solver == nil || query == nil {
		t.Fatal("ranked Product fixture")
	}
	state, status := solver.Solve(ctx)
	if called != 1 || state != nil || status != SolveCanceled {
		t.Fatalf("cancelled Product = state:%v status:%v calls:%d", state, status, called)
	}
	state, status = solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("staged Product leaked into later epoch: state:%v status:%v", state, status)
	}
	if value, readable := QueryResult(receipt, state); !readable || value != 1 || called < 2 {
		t.Fatalf("fresh Product result = value:%d readable:%v calls:%d", value, readable, called)
	}
}

// TestCancellationDuringQueryProjectionDropsUnpublishedResult cancels while
// ProjectRows owns a live query Product row. The query cannot return a frozen
// result, the solve must honestly report cancellation rather than an ordinary
// incomplete fixpoint, and a later epoch must materialize normally.
func TestCancellationDuringQueryProjectionDropsUnpublishedResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := 0
	solver, query, receipt, assembled := recurrenceSolverFixtureWithCallbacks(t, true, nil, nil, func(observation Observation, token QueryRead[OrderedCells[uint64]]) uint64 {
		called++
		if called > 1 {
			return recurrenceQueryValue(observation, token)
		}
		if !ProjectRows(observation, func(QueryRow) bool {
			cancel()
			return true
		}) {
			return 0
		}
		return 1
	})
	if !assembled || solver == nil || query == nil {
		t.Fatal("ranked Query fixture")
	}
	state, status := solver.Solve(ctx)
	if called != 1 || state != nil || status != SolveCanceled {
		t.Fatalf("cancelled Query = state:%v status:%v calls:%d", state, status, called)
	}
	state, status = solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("cancelled Query retained a frozen result: state:%v status:%v", state, status)
	}
	if value, readable := QueryResult(receipt, state); !readable || value != 1 || called != 2 {
		t.Fatalf("fresh Query result = value:%d readable:%v calls:%d", value, readable, called)
	}
}
