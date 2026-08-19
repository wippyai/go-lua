package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// TestExactObservationDerivesCommittedExactWriteCoordinate keeps observation
// geometry on the committed graph. Every observed member has one strong exact
// write, and every runtime observation points at an owned Group output.
func TestExactObservationDerivesCommittedExactWriteCoordinate(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 5, nil, nil)
	graph := fixture.graph.graph
	for index := 0; index < graph.GroupCount(); index++ {
		group, ok := graph.HyperedgeAt(index)
		if !ok || !graph.OwnsPoint(group.Output()) {
			t.Fatalf("group %d has no owned observation output", index)
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			write, writeOK := member.WriteAt(0)
			if !memberOK || !graph.OwnsMember(member) || member.WriteCount() != 1 || !writeOK || !write.Available() || write.Form != equation.SurfaceWriteExact || write.Mode != equation.TargetModeStrong || write.Local == 0 || write.Semantic.Available() || write.Normalizer.Available() {
				t.Fatalf("member %d did not retain one exact committed write", memberIndex)
			}
		}
	}
	if len(fixture.solver.runtime.observations) != len(fixture.observations) {
		t.Fatal("observation runtime/table cardinality diverged")
	}
	for index, observation := range fixture.solver.runtime.observations {
		if observation == nil || observation.observationID() != fixture.observations[index].ID || !graph.OwnsPoint(observation.observationPoint()) {
			t.Fatalf("observation %d lost its committed point or public ID", index)
		}
	}
}

// TestExactObservationReadsCommittedMemberAndRejectsForeignState proves an
// observation is readable only from the Solver and State that sealed its
// committed member table.
func TestExactObservationReadsCommittedMemberAndRejectsForeignState(t *testing.T) {
	firstSolver, firstObservation, firstState := newBorrowedObservationFixture(t)
	value, readable := testSnapshotObservationValue[uint64](firstSolver, firstState, firstObservation.ID)
	if !readable {
		t.Fatal("committed exact observation was not readable")
	}
	foreignSolver, _, foreignState := newBorrowedObservationFixture(t)
	if _, readable := testSnapshotObservationValue[uint64](foreignSolver, firstState, firstObservation.ID); readable {
		t.Fatal("foreign Solver read the committed observation")
	}
	if _, readable := testSnapshotObservationValue[uint64](firstSolver, foreignState, firstObservation.ID); readable {
		t.Fatal("foreign State crossed the observation fence")
	}
	if _, failure, ok := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil).graph.Seal([]ProgramObservationAdmission{firstObservation, firstObservation}); ok || !failure.Available() {
		t.Fatal("duplicate exact observation identity sealed")
	}
	if value == ^uint64(0) {
		t.Fatalf("fixture exact observation unexpectedly saturated: %d", value)
	}
}

// TestObservationPublicationRequiresOwnedObservationRow proves a solver with
// no observation inventory publishes no observation answer, while a sealed
// observation row publishes one stable value and can be explicitly detached.
func TestObservationPublicationRequiresOwnedObservationRow(t *testing.T) {
	querySolver, _, queryState := newBorrowedQueryFixture(t)
	if _, readable := testSnapshotObservationValue[uint64](querySolver, queryState, identity.ContentID{0xEE}); readable {
		t.Fatal("query-only publication manufactured an observation row")
	}
	solver, observation, state := newBorrowedObservationFixture(t)
	first, firstReadable := testSnapshotObservationValue[uint64](solver, state, observation.ID)
	second, secondReadable := testSnapshotObservationValue[uint64](solver, state, observation.ID)
	if !firstReadable || !secondReadable || first != second {
		t.Fatal("observation publication was not stable across borrowed reads")
	}
	published, ok := solver.PublishedSnapshot(state)
	if !ok {
		t.Fatal("observation publication unavailable")
	}
	answer, answerReadable := testSnapshotAnswer(solver, state, published.ObservationFamily(), observation.ID)
	owned, ownedOK := DetachAnswer[uint64](answer)
	if !answerReadable || !ownedOK || owned != first {
		t.Fatalf("detached exact observation=%d/%t", owned, ownedOK)
	}
	if _, solveStatus := solver.Solve(context.Background()); solveStatus != SolveComplete {
		t.Fatal("warm observation solve did not complete")
	}
}
