package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// regionPostfixLawEpoch builds one real executorEpoch over the same
// synthetic recurrence operandLawEpoch uses (a two-point back-edge, one
// region whose head is point 1), through the production buildOperandPlane
// and executorEpoch.operands.open paths. The region's points row is left
// empty on purpose: restartRegion's per-point reset loop needs a live
// carrier.RetainedWork this fixture does not build, and this law is about
// restartRegion's region-row field transition, not point publication. The
// region-row loop it exercises is identical for every subtree size.
func regionPostfixLawEpoch(t *testing.T) *executorEpoch {
	t.Helper()
	graph, _, _ := newRegionDischargeGraph(t)
	environments := []runtimeEnvironment{{index: 0, source: 0, target: 1}, {index: 1, source: 1, target: 1}}
	regions := []runtimeRegion{{
		active:              true,
		head:                1,
		parent:              schedule.NoRegion,
		environmentExternal: []int{0},
		environmentBack:     []int{1},
	}}
	plane, planed := buildOperandPlane(graph, nil, environments, factorSourceColumn{}, regions)
	if !planed {
		t.Fatal("region-postfix-law operand plane was not sealed")
	}
	runtime := &solverRuntime{
		graph:          graph,
		environments:   environments,
		regions:        regions,
		regionChildren: [][]int{nil},
		activeRegions:  []bool{true},
		pointRegion:    []int{schedule.NoRegion, 0},
		operands:       plane,
	}
	epoch := &executorEpoch{runtime: runtime, regions: make([]regionEpoch, 1), regionScratch: make([]int, 0, 1)}
	if !epoch.operands.open(plane) {
		t.Fatal("region-postfix-law epoch did not open over the sealed plane")
	}
	epoch.regions[0].phase = phaseAscent
	epoch.regions[0].episode = 1
	return epoch
}

// regionPostfixExactStateInvariant is the law itself: postfixAt != 0 implies
// hasExact, for every region row the epoch carries. It is checked, not
// assumed, at every point below the epoch reaches an observable state.
func regionPostfixExactStateInvariant(t *testing.T, epoch *executorEpoch, when string) {
	t.Helper()
	for index := range epoch.regions {
		region := epoch.regions[index]
		if region.postfixAt != 0 && !region.hasExact {
			t.Fatalf("%s: region[%d] postfixAt=%d but hasExact=false", when, index, region.postfixAt)
		}
	}
}

// TestRegionPostfixProofRequiresExactState pins the invariant that today
// holds only because restartRegion's two writes at
// runtime_region_interface.go sit three lines apart with nothing structural
// between them: a region certified by a nonzero postfixAt always still owns
// the exact carrier that certificate was proved against.
//
// Coverage: this drives the real restartRegion and rememberRegionPostfix
// production functions over a genuinely opened operand plane and region
// row, checking the invariant before, at, and after a real restart. It
// does not drive the invariant through the public Solve entry point: no
// fixture in this package reaches SolveDiagnosticRestart>0 through
// Solver.Solve/SolveWithDiagnostics (checked empirically against every
// receiptQueryMatrixFixture width and the guarded-cycle variant), and the
// package's only structurally cyclic fixture (newRegionDischargeGraph) is
// never wired to a full committed program, so it carries no carrier.Work a
// restart's per-point reset loop could run against. The per-point reset
// loop (runtime_region_interface.go's second half of restartRegion) is
// therefore not exercised by this test; only the region-row field
// transition -- the one the law is actually about -- is.
func TestRegionPostfixProofRequiresExactState(t *testing.T) {
	epoch := regionPostfixLawEpoch(t)
	regionPostfixExactStateInvariant(t, epoch, "fresh epoch")

	// Simulate the fold-side half of a proof: regionRHS/refreshPoint sets
	// hasExact through a real fold this fixture cannot run (it needs
	// carrier.Work), so this is the one hand-set field in the test. Every
	// field the invariant reasons about from here on is production-derived.
	epoch.regions[0].hasExact = true
	regionPostfixExactStateInvariant(t, epoch, "after simulated exact fold, before postfix proof")

	if !epoch.rememberRegionPostfix(0) {
		t.Fatal("region-postfix-law rememberRegionPostfix refused")
	}
	if epoch.regions[0].postfixAt == 0 {
		t.Fatal("region-postfix-law rememberRegionPostfix left postfixAt at zero")
	}
	regionPostfixExactStateInvariant(t, epoch, "after a real postfix proof")

	episodeBeforeRestart := epoch.regions[0].episode
	if !epoch.restartRegion(0, solveDiagnosticRestartHeadInterface, solveDiagnosticRestartInterfaceChanged, -1, carrier.RuleContribution{}) {
		t.Fatal("region-postfix-law restartRegion refused")
	}
	regionPostfixExactStateInvariant(t, epoch, "immediately after restartRegion")
	if epoch.regions[0].hasExact {
		t.Fatal("restartRegion left hasExact set on the freshly restarted episode")
	}
	if epoch.regions[0].postfixAt != 0 {
		t.Fatalf("restartRegion left postfixAt=%d on the freshly restarted episode, want 0", epoch.regions[0].postfixAt)
	}
	if epoch.regions[0].episode != episodeBeforeRestart+1 {
		t.Fatalf("restartRegion episode=%d, want %d", epoch.regions[0].episode, episodeBeforeRestart+1)
	}

	// Re-prove postfix for the new episode and check the invariant holds
	// across the reproof too, not only at the two endpoints.
	epoch.regions[0].hasExact = true
	regionPostfixExactStateInvariant(t, epoch, "after simulated exact fold in the new episode")
	if !epoch.rememberRegionPostfix(0) {
		t.Fatal("region-postfix-law rememberRegionPostfix refused in the new episode")
	}
	regionPostfixExactStateInvariant(t, epoch, "after the new episode's postfix proof")
}
