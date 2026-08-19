package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// scheduleStageRankFixture is one sealed mounted declaration whose template
// chains three native Call stages base -> dispatch -> summary -> effect. An
// offending placement is stated as an owner-declared environment edge, the one
// declaration surface that can reorder a composed schedule against the parent
// WTO certificate; the native stage placements themselves come from the sealed
// template and carry their own base-before-stage proof.
type scheduleStageRankFixture struct {
	constructedProgramFixture
}

func newScheduleStageRankFixture(t *testing.T) scheduleStageRankFixture {
	t.Helper()
	fixture := scheduleStageRankFixture{newConstructedProgramFixture(t)}
	if len(fixture.declaration.mounts) != 1 || fixture.declaration.mounts[0].template == nil {
		t.Fatal("schedule stage rank mount")
	}
	return fixture
}

// pointRef is the dense Point address of one mounted template point. The
// construction walks the mounts in order and each template's point stream in
// index order, so under a single mount the dense address is the template index.
func (fixture scheduleStageRankFixture) pointRef(t *testing.T, reusable identity.ContentID) equation.PointRef {
	t.Helper()
	template := fixture.declaration.mounts[0].template
	for index := 0; index < template.PointCount(); index++ {
		row, rowOK := template.PointAt(index)
		if !rowOK {
			t.Fatalf("schedule stage rank template point %d", index)
		}
		if row.ID == reusable {
			return equation.PointAt(index)
		}
	}
	t.Fatalf("schedule stage rank point %v", reusable)
	return 0
}

// stageInversion is the owner-declared environment edge that orders the Point a
// native Call stage is staged from after the stage Point itself. One schedule
// cannot satisfy that edge and the stage order at once, so the placement at
// stage offends the composition gate.
func (fixture scheduleStageRankFixture) stageInversion(t *testing.T, stage, input identity.ContentID, salt uint64) equation.EnvironmentEdge {
	t.Helper()
	source := fixture.declaration.sites.mounted[artifactMountedPoint{mount: fixture.owner.mount, reusable: stage}]
	target := fixture.declaration.sites.mounted[artifactMountedPoint{mount: fixture.owner.mount, reusable: input}]
	reindex, reindexOK := ruleInputReindex(source.Scope(), target.Scope())
	boundary := equation.BoundaryInput(source, target, compositionKeyOf(coldKey(salt)), equation.TrueExpr(), reindex, equation.TrueExpr())
	if !source.Available() || !target.Available() || !reindexOK || !boundary.Available() {
		t.Fatal("schedule stage rank inversion edge")
	}
	return equation.EnvironmentEdge{Target: fixture.pointRef(t, input), Input: boundary}
}

// construct folds the declaration with the offending edges appended.
func (fixture scheduleStageRankFixture) construct(edges []equation.EnvironmentEdge) (constructedTopology, topologyConstructionRefusal) {
	declaration := fixture.declaration
	declaration.environmentEdges = edges
	return constructTopology(declaration)
}

// TestConstructedScheduleStageRankIsTotalOverOffendingPlacements proves the
// reported stage rank is the minimum over every offending placement. The
// placements live in an unordered map, so a first-offender report would change
// between repeated constructions of one identical declaration.
func TestConstructedScheduleStageRankIsTotalOverOffendingPlacements(t *testing.T) {
	fixture := newScheduleStageRankFixture(t)
	clean, cleanRefusal := fixture.construct(nil)
	if cleanRefusal.Available() || !clean.Available() {
		t.Fatalf("clean declaration stage=%v step=%v ordinal=%d", cleanRefusal.Stage(), cleanRefusal.Step(), cleanRefusal.Ordinal())
	}
	first := fixture.stageInversion(t, fixture.owner.dispatch, fixture.owner.base, 947_401)
	second := fixture.stageInversion(t, fixture.owner.effect, fixture.owner.summary, 947_402)
	firstConstructed, firstRefusal := fixture.construct([]equation.EnvironmentEdge{first})
	if firstConstructed.Available() || firstRefusal.Stage() != ProgramConstructionStageTopologySeal || firstRefusal.Step() != topologyConstructionStepSchedule {
		t.Fatalf("first offender stage=%v step=%v published=%t", firstRefusal.Stage(), firstRefusal.Step(), firstConstructed.Available())
	}
	secondConstructed, secondRefusal := fixture.construct([]equation.EnvironmentEdge{second})
	if secondConstructed.Available() || secondRefusal.Stage() != ProgramConstructionStageTopologySeal || secondRefusal.Step() != topologyConstructionStepSchedule {
		t.Fatalf("second offender stage=%v step=%v published=%t", secondRefusal.Stage(), secondRefusal.Step(), secondConstructed.Available())
	}
	firstRank, secondRank := firstRefusal.Ordinal(), secondRefusal.Ordinal()
	if firstRank == secondRank {
		t.Fatalf("offender ranks coincide at %d", firstRank)
	}
	expected := firstRank
	if secondRank < expected {
		expected = secondRank
	}
	both := []equation.EnvironmentEdge{first, second}
	for repeat := 0; repeat < 64; repeat++ {
		constructed, refusal := fixture.construct(both)
		if constructed.Available() || refusal.Stage() != ProgramConstructionStageTopologySeal || refusal.Step() != topologyConstructionStepSchedule {
			t.Fatalf("repeat %d stage=%v step=%v published=%t", repeat, refusal.Stage(), refusal.Step(), constructed.Available())
		}
		if refusal.Ordinal() != expected {
			t.Fatalf("repeat %d rank %d, minimum %d", repeat, refusal.Ordinal(), expected)
		}
	}
}
