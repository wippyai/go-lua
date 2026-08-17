package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

type scheduleStageRankFixture struct {
	sealed   *artifactReceiptTopology
	topology *equation.Topology
	graph    *equation.Graph
	mount    identity.ContentID
}

func newScheduleStageRankFixture(t *testing.T) scheduleStageRankFixture {
	t.Helper()
	owner := newBootstrapTransportLawOwner(t)
	artifact := newBootstrapTransportLawArtifact(t, owner.schema, 8)
	mount := bootstrapTransportLawID(8, 30)
	mounted, mountedOK := NewMountedArtifactReceipt(artifact.receipt, mount)
	if !mountedOK {
		t.Fatal("schedule stage rank mount")
	}
	witness := bootstrapTransportLawWitness(t, owner, 8)
	topology, graph := commitBootstrapTransportLaw(t, owner.binding, []MountedArtifactReceipt{mounted}, witness)
	if topology.artifact == nil || topology.topology == nil || graph.graph == nil {
		t.Fatal("schedule stage rank topology")
	}
	if len(topology.artifact.points) < 2 {
		t.Fatalf("schedule stage rank points %d", len(topology.artifact.points))
	}
	return scheduleStageRankFixture{sealed: topology.artifact, topology: topology.topology, graph: graph.graph, mount: mount}
}

// replay rebuilds the pre-seal receipt rows so the composition gate can be
// re-run against the same sealed topology with an additional stage placement
// catalog.
func (fixture scheduleStageRankFixture) replay(offenders map[artifactMountedRuleOccurrence]artifactNativeCallStage) *artifactReceiptTopology {
	replay := *fixture.sealed
	replay.sealed = nil
	replay.callStages = make(map[artifactMountedRuleOccurrence]artifactNativeCallStage, len(fixture.sealed.callStages)+len(offenders))
	for key, value := range fixture.sealed.callStages {
		replay.callStages[key] = value
	}
	for key, value := range offenders {
		replay.callStages[key] = value
	}
	return &replay
}

// unorderedStage is a native Call stage whose base and stage points coincide,
// so the placement can never satisfy the strict base-before-stage order.
func (fixture scheduleStageRankFixture) unorderedStage(point identity.ContentID, occurrence byte) (artifactMountedRuleOccurrence, artifactNativeCallStage) {
	key := artifactMountedRuleOccurrence{mount: fixture.mount, occurrence: bootstrapTransportLawID(8, occurrence)}
	return key, artifactNativeCallStage{stage: rows.ArtifactRuleStageCallDispatch, point: point, input: point, mountedPoint: point, mountedInput: point}
}

func (fixture scheduleStageRankFixture) validate(t *testing.T, offenders map[artifactMountedRuleOccurrence]artifactNativeCallStage) (ReceiptScheduleFailure, uint32, bool) {
	t.Helper()
	return validateMountedArtifactSchedule(fixture.replay(offenders), fixture.topology, fixture.graph)
}

// TestStructuralScheduleStageRankIsTotalOverOffendingPlacements proves the
// reported stage rank is the minimum over every offending placement. The
// placements live in an unordered map, so a first-offender report would change
// between repeated validations of one identical receipt.
func TestStructuralScheduleStageRankIsTotalOverOffendingPlacements(t *testing.T) {
	fixture := newScheduleStageRankFixture(t)
	if failure, rank, ok := fixture.validate(t, nil); !ok || failure != ReceiptScheduleFailureNone || rank != 0 {
		t.Fatalf("clean receipt failure=%d rank=%d ok=%v", failure, rank, ok)
	}
	firstKey, firstStage := fixture.unorderedStage(fixture.sealed.points[0], 70)
	secondKey, secondStage := fixture.unorderedStage(fixture.sealed.points[1], 71)
	failure, firstRank, ok := fixture.validate(t, map[artifactMountedRuleOccurrence]artifactNativeCallStage{firstKey: firstStage})
	if ok || failure != ReceiptScheduleFailureStage {
		t.Fatalf("first offender failure=%d ok=%v", failure, ok)
	}
	failure, secondRank, ok := fixture.validate(t, map[artifactMountedRuleOccurrence]artifactNativeCallStage{secondKey: secondStage})
	if ok || failure != ReceiptScheduleFailureStage {
		t.Fatalf("second offender failure=%d ok=%v", failure, ok)
	}
	if firstRank == secondRank {
		t.Fatalf("offender ranks coincide at %d", firstRank)
	}
	expected := firstRank
	if secondRank < expected {
		expected = secondRank
	}
	both := map[artifactMountedRuleOccurrence]artifactNativeCallStage{firstKey: firstStage, secondKey: secondStage}
	for repeat := 0; repeat < 64; repeat++ {
		failure, rank, ok := fixture.validate(t, both)
		if ok || failure != ReceiptScheduleFailureStage {
			t.Fatalf("repeat %d failure=%d ok=%v", repeat, failure, ok)
		}
		if rank != expected {
			t.Fatalf("repeat %d rank %d, minimum %d", repeat, rank, expected)
		}
	}
}
