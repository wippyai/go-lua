package engine

import (
	"context"
	"testing"
)

// TestCommittedProgramSealMintsIndependentSolvers states the current revision
// boundary: a committed program is immutable, while each Seal binds a fresh
// runtime plane and mints one independent Solver.
func TestCommittedProgramSealMintsIndependentSolvers(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	first, firstFailure, firstOK := fixture.graph.Seal(nil)
	second, secondFailure, secondOK := fixture.graph.Seal(nil)
	if !firstOK || !secondOK || first == nil || second == nil {
		t.Fatalf("seals = %v/%v/%v and %v/%v/%v", first, firstFailure, firstOK, second, secondFailure, secondOK)
	}
	if first == second || first.store == second.store || first.runtime == second.runtime {
		t.Fatal("two program seals shared mutable Solver state")
	}
	if first.runtime.graph != second.runtime.graph || first.runtime.topology != second.runtime.topology || first.runtime.graph != fixture.graph.graph {
		t.Fatal("a revision rebound different committed geometry")
	}
	for _, runtime := range []*solverRuntime{first.runtime, second.runtime} {
		if !runtime.artifactBacked || !runtime.contexts.Available() ||
			!runtime.contextIndex.OwnedBy(runtime.contexts, runtime.graph.PointCount(), runtime.contextLayout.Generation()) ||
			!runtime.contextLayout.OwnedBy(runtime.contextIndex, runtime.contexts, runtime.pointOwners, runtime.contextLayout.Generation()) ||
			len(runtime.pointOwners) != runtime.graph.PointCount() || runtime.executionPlan == nil || !runtime.executionPlan.Available() ||
			runtime.executionPlan.Graph() != runtime.graph || runtime.executionPlan.Generation() != runtime.contextLayout.Generation() ||
			runtime.executionPlan.StateCount() != runtime.contextLayout.StateCount() {
			t.Fatal("sealed runtime dropped or rebound the committed execution-context plane")
		}
	}
	if first.runtime.executionPlan == second.runtime.executionPlan {
		t.Fatal("two program seals shared one execution-plan owner")
	}
	if &first.runtime.pointOwners[0] == &fixture.graph.pointOwners[0] || &second.runtime.pointOwners[0] == &fixture.graph.pointOwners[0] {
		t.Fatal("sealed runtime retained the committed point-owner slice")
	}
}

// TestActivationRevisionRebindsFromSealedInputsOnly proves a second Seal
// consumes only the committed graph, schema authority and declared tables; it
// does not depend on the first Solver's mutable relation or publication.
func TestActivationRevisionRebindsFromSealedInputsOnly(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	first, _, firstOK := fixture.graph.Seal(nil)
	if !firstOK || first == nil {
		t.Fatal("first sealed runtime")
	}
	if _, status := first.Solve(context.Background()); status != SolveComplete {
		t.Fatalf("first solve status=%v", status)
	}
	second, failure, secondOK := fixture.graph.Seal(nil)
	if !secondOK || second == nil {
		t.Fatalf("second sealed runtime failure=%v", failure)
	}
	if second.runtime.graph != fixture.graph.graph || second.runtime.publication == first.runtime.publication {
		t.Fatal("second Seal retained first publication state")
	}
	if second.completion != 0 || second.lastSolved.Available() {
		t.Fatal("second Seal inherited the first Solver lifecycle")
	}
}

// TestRealActivationRevisionCompletesOverTheSupersededProgram proves the
// superseded Solver remains a valid completed publication while a fresh Seal
// solves the same immutable committed program independently.
func TestRealActivationRevisionCompletesOverTheSupersededProgram(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	first, _, firstOK := fixture.graph.Seal(nil)
	if !firstOK || first == nil {
		t.Fatal("first sealed runtime")
	}
	oldState, oldStatus := first.Solve(context.Background())
	if oldState == nil || oldStatus != SolveComplete {
		t.Fatalf("superseded solve = state:%t status:%v", oldState != nil, oldStatus)
	}
	second, failure, secondOK := fixture.graph.Seal(nil)
	if !secondOK || second == nil {
		t.Fatalf("revision seal failure=%v", failure)
	}
	newState, newStatus := second.Solve(context.Background())
	if newState == nil || newStatus != SolveComplete {
		t.Fatalf("revision solve = state:%t status:%v", newState != nil, newStatus)
	}
	if !first.ownsCompletedState(oldState) || !second.ownsCompletedState(newState) || first.ownsCompletedState(newState) || second.ownsCompletedState(oldState) {
		t.Fatal("publication ownership crossed the Solver revision fence")
	}
}
