package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestSolverPublicationStampsFenceCompletedResults proves the two Solver
// fences that share one Generation discipline. The completion serial orders
// published results and never rewinds; the activation-relation stamp binds a
// result to the exact relation that produced it, so publishing a later relation
// invalidates every earlier State without touching the State itself.
func TestSolverPublicationStampsFenceCompletedResults(t *testing.T) {
	solver, _ := newDiagnosticsReceiptSolver(t, false)
	if solver == nil || !solver.relation.Available() {
		t.Fatal("solver publication")
	}
	sealed := solver.relation
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil || state.completion == nil {
		t.Fatalf("solve status=%v state=%t", status, state != nil)
	}
	if !state.completion.serial.Available() || state.completion.serial != solver.completion {
		t.Fatalf("completion serial %d did not name the solver's published completion %d", state.completion.serial, solver.completion)
	}
	if state.completion.relation != solver.relation.Generation() {
		t.Fatal("completed state was not stamped with the live activation relation")
	}
	if !solver.ownsCompletedState(state) {
		t.Fatal("solver rejected its own freshly published state")
	}

	// A later completion serial never resurrects an earlier result, and it never
	// rewinds: atOrBefore is the whole comparison.
	if atOrBefore(state.completion.serial.Next(), solver.completion) {
		t.Fatal("a future completion serial passed the publication fence")
	}
	if !atOrBefore(state.completion.serial, solver.completion.Next()) {
		t.Fatal("a retained completion serial failed the publication fence")
	}
	var unset identity.Generation
	if atOrBefore(unset, solver.completion) || atOrBefore(state.completion.serial, unset) {
		t.Fatal("an unset stamp passed the publication fence")
	}

	// Publishing the next activation relation is the one act that invalidates a
	// retained result. The State is untouched; only the stamp comparison changes.
	published, publishedOK := solver.runtime.topology.Publish(sealed, sealed.Rows())
	if !publishedOK || !sealed.Precedes(published) {
		t.Fatal("solver topology did not advance its publication")
	}
	solver.relation = published
	if solver.ownsCompletedState(state) {
		t.Fatal("a state from a superseded activation relation stayed owned")
	}
	solver.relation = sealed
	if !solver.ownsCompletedState(state) {
		t.Fatal("restoring the publishing relation did not restore the result")
	}
}

// TestExecutionStampCellsAdmitOnlyTheirLiveStamp proves the discipline shared
// by every live-execution fence in the engine: a cell admits exactly one stamp,
// an unavailable stamp is admitted by nothing, a nested token can be claimed
// only while the cell is free, and revoking a stamp that is not live changes
// nothing.
func TestExecutionStampCellsAdmitOnlyTheirLiveStamp(t *testing.T) {
	var sequence generationSequence
	first, issued := sequence.issue()
	second, reissued := sequence.issue()
	if !issued || !reissued || !first.Precedes(second) || second != first.Next() {
		t.Fatalf("sequence did not advance monotonically: first=%d second=%d", first, second)
	}

	var cell generationCell
	if cell.live().Available() || cell.holds(0) || cell.holds(first) {
		t.Fatal("a free cell admitted a stamp")
	}
	if cell.revoke(0) || cell.revoke(first) {
		t.Fatal("a free cell revoked a stamp")
	}
	if !cell.claim(first) || cell.claim(second) || cell.claim(first) {
		t.Fatal("a claimed cell admitted a second holder")
	}
	if !cell.holds(first) || cell.holds(second) || cell.holds(0) {
		t.Fatal("a claimed cell admitted a foreign stamp")
	}
	if cell.revoke(second) || !cell.holds(first) {
		t.Fatal("a foreign stamp revoked a live holder")
	}
	if !cell.revoke(first) || cell.holds(first) || cell.live().Available() {
		t.Fatal("revoking the live stamp did not free the cell")
	}

	cell.open(second)
	if !cell.holds(second) || cell.holds(first) {
		t.Fatal("opening a cell did not install exactly its stamp")
	}
	next, advanced := cell.advance()
	if !advanced || next != second.Next() || !cell.holds(next) || cell.holds(second) {
		t.Fatal("advancing a cell did not supersede the previous stamp")
	}
}
