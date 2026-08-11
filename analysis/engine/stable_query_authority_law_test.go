package engine

import (
	"context"
	"testing"
)

// TestQueryResultsRemainBoundToDeclaredQueries uses a deliberately shuffled
// graph and declaration order.  The only contract a caller can observe is
// that every declared Query reads its own completed value.
func TestQueryResultsRemainBoundToDeclaredQueries(t *testing.T) {
	fixture := buildStaticMatrixFixture(t, 16, staticMatrixPermutations(16)[1])
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("matrix solve = state:%v status:%v", state, status)
	}

	for index := range fixture.queries {
		got, readable := QueryResult(fixture.receipts[index], state)
		want := staticMatrixValue(index, 16)
		if !readable || got != want {
			t.Fatalf("QueryResult[%d] = %d/%v, want %d/true", index, got, readable, want)
		}
	}
}

// TestCompletedStatesStayReadableAcrossWarmSolve keeps result authority at
// the public Solver/Query boundary: an earlier completed State remains a
// valid immutable observation after the Solver serves a warm result.
func TestCompletedStatesStayReadableAcrossWarmSolve(t *testing.T) {
	callbacks := &lifecycleLawCallbacks{}
	solver, receipt := newLifecycleLawSolver(t, callbacks)

	first, status := solver.Solve(context.Background())
	if status != SolveComplete || first == nil {
		t.Fatalf("first solve = state:%v status:%v", first, status)
	}
	firstValue, readable := QueryResult(receipt, first)
	if !readable || firstValue != 1 {
		t.Fatalf("first result = %d/%v", firstValue, readable)
	}
	beforeTransfers, beforeProjects, beforeFreezes := callbacks.transfers, callbacks.projects, callbacks.freezes

	second, status := solver.Solve(context.Background())
	if status != SolveComplete || second == nil {
		t.Fatalf("warm solve = state:%v status:%v", second, status)
	}
	if got, readable := QueryResult(receipt, first); !readable || got != firstValue {
		t.Fatalf("first result after warm solve = %d/%v", got, readable)
	}
	if got, readable := QueryResult(receipt, second); !readable || got != firstValue {
		t.Fatalf("warm result = %d/%v", got, readable)
	}
	if callbacks.transfers != beforeTransfers || callbacks.projects != beforeProjects || callbacks.freezes != beforeFreezes {
		t.Fatalf("warm solve reran callbacks: before=%d/%d/%d after=%d/%d/%d", beforeTransfers, beforeProjects, beforeFreezes, callbacks.transfers, callbacks.projects, callbacks.freezes)
	}
	allocations := testing.AllocsPerRun(100, func() {
		warm, warmStatus := solver.Solve(context.Background())
		if warmStatus != SolveComplete || warm == nil {
			panic("warm receipt solve")
		}
		if value, readable := QueryResult(receipt, warm); !readable || value != firstValue {
			panic("warm receipt result")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm receipt solve allocations = %v, want 0", allocations)
	}
}

// TestQueryResultRejectsForeignAndCanceledStates verifies the public failure
// modes without manufacturing or inspecting State internals.
func TestQueryResultRejectsForeignAndCanceledStates(t *testing.T) {
	left, leftReceipt := newLifecycleLawSolver(t, &lifecycleLawCallbacks{})
	right, rightReceipt := newLifecycleLawSolver(t, &lifecycleLawCallbacks{})

	leftState, status := left.Solve(context.Background())
	if status != SolveComplete || leftState == nil {
		t.Fatalf("left solve = state:%v status:%v", leftState, status)
	}
	rightState, status := right.Solve(context.Background())
	if status != SolveComplete || rightState == nil {
		t.Fatalf("right solve = state:%v status:%v", rightState, status)
	}
	if _, readable := QueryResult(leftReceipt, rightState); readable {
		t.Fatal("foreign query read a completed state")
	}
	if _, readable := QueryResult(rightReceipt, leftState); readable {
		t.Fatal("query read a foreign solver state")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled, status := left.Solve(ctx)
	if canceled != nil || status != SolveCanceled {
		t.Fatalf("canceled solve = state:%v status:%v", canceled, status)
	}
	if _, readable := QueryResult(leftReceipt, canceled); readable {
		t.Fatal("canceled solve exposed a query result")
	}
}
