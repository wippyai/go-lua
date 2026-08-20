package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// frontierCarryEpoch is one epoch over a synthetic recurrence whose region
// rows a second frontier can extend, re-point or leave alone. Point 1 is the
// region head; environment edge 0 is its external ingress, environment edge 1
// its back ingress, and factor edge 0 a second external ingress whose source
// a replacement can move.
func frontierCarryEpoch(t *testing.T, regions []runtimeRegion, environments []runtimeEnvironment, factors []runtimeFactorEdge) *executorEpoch {
	t.Helper()
	graph, _, _ := newRegionDischargeGraph(t)
	plane, planed := buildOperandPlane(graph, nil, environments, installedFactorSources(factors), regions)
	if !planed {
		t.Fatal("operand plane was not sealed")
	}
	runtime := &solverRuntime{
		graph:          graph,
		environments:   environments,
		factorEdges:    factors,
		regions:        regions,
		regionChildren: make([][]int, len(regions)),
		activeRegions:  []bool{true},
		pointRegion:    []int{schedule.NoRegion, 0},
		operands:       plane,
	}
	epoch := &executorEpoch{runtime: runtime, regions: make([]regionEpoch, len(regions))}
	if !epoch.operands.open(plane) {
		t.Fatal("operand epoch did not open over the sealed plane")
	}
	return epoch
}

func frontierCarryRegions(externalEnvironments, factorExternal []int) []runtimeRegion {
	return []runtimeRegion{{
		active:              true,
		head:                1,
		parent:              schedule.NoRegion,
		points:              []int{1},
		environmentExternal: externalEnvironments,
		environmentBack:     []int{1},
		factorExternal:      factorExternal,
	}}
}

func frontierCarryEnvironments() []runtimeEnvironment {
	return []runtimeEnvironment{{index: 0, source: 0, target: 1}, {index: 1, source: 1, target: 1}, {index: 2, source: 0, target: 1}}
}

// installFrontierCarry replays the two operations the activation commit
// performs over an already running epoch: it carries the region episodes under
// the frontier's change-fact, then re-opens the operand plane over the rows
// that frontier publishes and marks what it installed.
func installFrontierCarry(t *testing.T, epoch *executorEpoch, next []runtimeRegion, factors []runtimeFactorEdge, repointed map[int]struct{}) []regionRowCarry {
	t.Helper()
	previous := epoch.runtime.regions
	active := epoch.runtime.activeRegions
	previousOf, carry, classified := regionFrontierCarry(previous, next, active, active, repointed)
	if !classified {
		t.Fatal("the frontier was not classified against the running rows")
	}
	plane, planed := buildOperandPlane(epoch.runtime.graph, nil, epoch.runtime.environments, installedFactorSources(factors), next)
	if !planed {
		t.Fatal("the frontier operand plane was not sealed")
	}
	fresh := make([]regionEpoch, len(next))
	for index := range fresh {
		fresh[index] = regionEpoch{phase: phaseAscent, episode: 1, invalid: true}
	}
	epoch.runtime.regions, epoch.runtime.factorEdges, epoch.runtime.operands = next, factors, plane
	carried, ok := epoch.carryRegionEpisodes(fresh, previousOf, carry)
	if !ok {
		t.Fatal("the frontier region episodes were not carried")
	}
	epoch.regions = carried
	if !epoch.markCarriedRegionOperands(epoch.operands.openAdmitted(plane, previousOf, carry), carry) {
		t.Fatal("the installed operands were not marked")
	}
	return carry
}

// settleFrontierCarryEpisode puts the region in the state an activation finds
// it in: one narrow episode holding the exact row its operands folded to, with
// every mark it has taken already folded into that row.
func settleFrontierCarryEpisode(epoch *executorEpoch) *regionEpoch {
	state := &epoch.regions[0]
	state.phase = phaseNarrow
	state.episode = 3
	state.exact, state.hasExact = carrier.PointRHS{}, true
	state.rememberAt = epoch.operands.advance()
	state.pending = change.Classified()
	return state
}

// sameFrontierCarryEpisode compares the episode fields the carry is stated
// over. The two carrier rows are opaque handles, so they are compared through
// the same equality the epoch reads them with.
func sameFrontierCarryEpisode(left, right regionEpoch) bool {
	return left.phase == right.phase && left.episode == right.episode &&
		left.hasExact == right.hasExact && sameFrontierCarryRow(left.exact, right.exact) &&
		left.hasAccumulator == right.hasAccumulator && sameFrontierCarryRow(left.accumulator, right.accumulator) &&
		left.invalid == right.invalid && left.interfaceRefreshPending == right.interfaceRefreshPending &&
		left.rememberAt == right.rememberAt && left.externalAt == right.externalAt && left.backAt == right.backAt &&
		left.pointsAt == right.pointsAt && left.enterAt == right.enterAt && left.postfixAt == right.postfixAt &&
		left.pending == right.pending
}

func sameFrontierCarryRow(left, right carrier.PointRHS) bool {
	return left.Valid() == right.Valid() && left.Scope() == right.Scope()
}

// TestFrontierCarriesTheEpisodeItsRowsDoNotChange is the snapshot-collapse
// law for a frontier installation. A region whose operand rows the frontier
// leaves alone is holding a row that still folds from exactly its operands, so
// the installation may not collapse it: the episode, its exact row, its
// remember stamp and its evidence axis are the ones it settled with.
func TestFrontierCarriesTheEpisodeItsRowsDoNotChange(t *testing.T) {
	regions := frontierCarryRegions([]int{0}, []int{0})
	factors := []runtimeFactorEdge{{index: 0, source: 0, target: 1}}
	epoch := frontierCarryEpoch(t, regions, frontierCarryEnvironments(), factors)
	settled := *settleFrontierCarryEpisode(epoch)

	carry := installFrontierCarry(t, epoch, frontierCarryRegions([]int{0}, []int{0}), factors, nil)
	if carry[0] != regionRowRetained {
		t.Fatalf("an unchanged region classified as carry=%d", carry[0])
	}
	if !sameFrontierCarryEpisode(epoch.regions[0], settled) {
		t.Fatalf("an unchanged region lost its episode: %+v want %+v", epoch.regions[0], settled)
	}
	if epoch.regionExactInputsChanged(0) {
		t.Fatal("an unchanged region reported changed exact inputs across the installation")
	}
}

// TestFrontierMarkSurvivesTheInstallation is the same law on the tick space
// the delta path reads. A mark taken before an installation is evidence the
// region has not folded yet; re-deriving the plane may not lose it, because a
// lost mark reads as an unchanged operand and silently drops a term from the
// next refold.
func TestFrontierMarkSurvivesTheInstallation(t *testing.T) {
	regions := frontierCarryRegions([]int{0}, []int{0})
	factors := []runtimeFactorEdge{{index: 0, source: 0, target: 1}}
	epoch := frontierCarryEpoch(t, regions, frontierCarryEnvironments(), factors)
	settleFrontierCarryEpisode(epoch)
	at := epoch.regions[0].rememberAt
	if !epoch.markSourceOperands(0, change.Set{Reasons: change.ChangedUnit, Direction: change.Known | change.Ascends}) {
		t.Fatal("marking the external source was refused")
	}
	begin, end, ok := epoch.operands.plane.regionWindow(0, operandExternalEnvironment)
	if !ok || end-begin != 1 || !epoch.operands.changedSince(uint32(begin), at) {
		t.Fatal("the external environment operand was not marked before the installation")
	}

	installFrontierCarry(t, epoch, frontierCarryRegions([]int{0}, []int{0}), factors, nil)

	begin, end, ok = epoch.operands.plane.regionWindow(0, operandExternalEnvironment)
	if !ok || end-begin != 1 {
		t.Fatalf("the installed external environment window is [%d,%d)", begin, end)
	}
	if !epoch.operands.changedSince(uint32(begin), at) {
		t.Fatal("an operand marked before the installation reads as unchanged after it")
	}
}

// TestFrontierExtensionKeepsTheRowItAscends states the extension half. An
// appended operand is a join term the row did not have, so the row ascends and
// the settled fold is still the lower bound the next delta folds onto: the new
// ascent episode keeps it as its accumulator, the appended operand reads as
// changed, and the evidence axis admits the reuse.
func TestFrontierExtensionKeepsTheRowItAscends(t *testing.T) {
	regions := frontierCarryRegions([]int{0}, []int{0})
	factors := []runtimeFactorEdge{{index: 0, source: 0, target: 1}}
	epoch := frontierCarryEpoch(t, regions, frontierCarryEnvironments(), factors)
	settled := *settleFrontierCarryEpisode(epoch)

	carry := installFrontierCarry(t, epoch, frontierCarryRegions([]int{0, 2}, []int{0}), factors, nil)
	if carry[0] != regionRowExtended {
		t.Fatalf("an appended row classified as carry=%d", carry[0])
	}
	state := &epoch.regions[0]
	if state.phase != phaseAscent || state.episode != settled.episode+1 {
		t.Fatalf("an extended region opened phase=%d episode=%d", state.phase, state.episode)
	}
	if !state.hasAccumulator || !sameFrontierCarryRow(state.accumulator, settled.exact) {
		t.Fatalf("an extended region dropped the row it settled: hasAccumulator=%t", state.hasAccumulator)
	}
	if !regionAccumulatorEvidenceAdmits(state) {
		t.Fatalf("an ascending extension refused its own accumulator: %+v", state.pending)
	}
	if state.externalAt <= state.rememberAt {
		t.Fatal("an appended external operand did not reach the region's ingress stamp")
	}
	begin, end, ok := epoch.operands.plane.regionWindow(0, operandExternalEnvironment)
	if !ok || end-begin != 2 {
		t.Fatalf("the installed external environment window is [%d,%d)", begin, end)
	}
	if !epoch.operands.changedSince(uint32(begin+1), state.rememberAt) {
		t.Fatal("an appended operand reads as unchanged")
	}
	if epoch.operands.changedSince(uint32(begin), state.rememberAt) {
		t.Fatal("an operand the frontier did not touch reads as changed")
	}
}

// TestFrontierRepointRefusesReuseByEvidence is the refusal half, and it states
// how a drop must happen: an operand whose source the frontier moved is
// classified by nobody, so it reaches the region's evidence axis unclassified
// and Admits refuses the retained accumulator. No lifecycle path throws the
// accumulator away behind the predicate's back.
func TestFrontierRepointRefusesReuseByEvidence(t *testing.T) {
	regions := frontierCarryRegions([]int{0}, []int{0})
	factors := []runtimeFactorEdge{{index: 0, source: 0, target: 1}}
	epoch := frontierCarryEpoch(t, regions, frontierCarryEnvironments(), factors)
	settleFrontierCarryEpisode(epoch)

	repointed := []runtimeFactorEdge{{index: 0, source: 1, target: 1}}
	carry := installFrontierCarry(t, epoch, frontierCarryRegions([]int{0}, []int{0}), repointed, map[int]struct{}{0: {}})
	if carry[0] != regionRowRebuilt {
		t.Fatalf("a re-pointed row classified as carry=%d", carry[0])
	}
	state := &epoch.regions[0]
	if !state.pending.Unknown() {
		t.Fatalf("a re-pointed operand reached the evidence axis classified: %+v", state.pending)
	}
	if regionAccumulatorEvidenceAdmits(state) {
		t.Fatal("a re-pointed row admitted a reuse its evidence refuses")
	}
	begin, end, ok := epoch.operands.plane.regionWindow(0, operandExternalFactor)
	if !ok || end-begin != 1 || !epoch.operands.changedSince(uint32(begin), state.rememberAt) {
		t.Fatal("a re-pointed operand does not read as changed")
	}
}
