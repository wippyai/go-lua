package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// operandLawEpoch builds one epoch over a synthetic recurrence: two points,
// one region whose head is point 1, one external environment edge sourced at
// point 0 and one back environment edge sourced at point 1.
func operandLawEpoch(t *testing.T) *executorEpoch {
	t.Helper()
	graph, _, _ := newRegionDischargeGraph(t)
	if graph.PointCount() != 2 || graph.GroupCount() != 0 {
		t.Fatalf("operand-law graph points=%d groups=%d", graph.PointCount(), graph.GroupCount())
	}
	environments := []runtimeEnvironment{{index: 0, source: 0, target: 1}, {index: 1, source: 1, target: 1}}
	regions := []runtimeRegion{{
		active:              true,
		head:                1,
		parent:              schedule.NoRegion,
		points:              []int{1},
		environmentExternal: []int{0},
		environmentBack:     []int{1},
	}}
	plane, planed := buildOperandPlane(graph, nil, environments, factorSourceColumn{}, regions)
	if !planed {
		t.Fatal("operand plane was not sealed")
	}
	runtime := &solverRuntime{graph: graph, environments: environments, regions: regions, activeRegions: []bool{true}, pointRegion: []int{schedule.NoRegion, 0}, operands: plane}
	epoch := &executorEpoch{runtime: runtime, regions: make([]regionEpoch, 1)}
	if !epoch.operands.open(plane) {
		t.Fatal("operand epoch did not open over the sealed plane")
	}
	return epoch
}

// TestOperandPlaneWindowsPartitionTheFusedPlane proves the forward directory
// is a partition: every region row is a half-open window of exactly its own
// width, and the ordinal a window opens at recovers that row and position.
func TestOperandPlaneWindowsPartitionTheFusedPlane(t *testing.T) {
	epoch := operandLawEpoch(t)
	plane := epoch.operands.plane
	widths := map[operandKind]int{operandExternalEnvironment: 1, operandBackEnvironment: 1, operandRegionPoint: 1}
	covered := 0
	for kind := operandKind(0); kind < operandKindCount; kind++ {
		begin, end, ok := plane.regionWindow(0, kind)
		if !ok {
			t.Fatalf("region window %d was refused", kind)
		}
		if end-begin != widths[kind] {
			t.Fatalf("region window %d width=%d want %d", kind, end-begin, widths[kind])
		}
		covered += end - begin
		for position := begin; position < end; position++ {
			region, recovered, offset, ok := plane.operandRegion(uint32(position))
			if !ok || region != 0 || recovered != kind || offset != position-begin {
				t.Fatalf("ordinal %d recovered region=%d kind=%d position=%d ok=%t", position, region, recovered, offset, ok)
			}
		}
	}
	if covered != plane.total {
		t.Fatalf("region windows cover %d of %d fused ordinals", covered, plane.total)
	}
}

// TestOperandMarkReachesExactlyTheRowsItsSourceMints proves the transpose is
// the inverse of the forward rows: publishing a Point stamps every operand
// sourced at it, in the region row that owns it, and stamps nothing else.
func TestOperandMarkReachesExactlyTheRowsItsSourceMints(t *testing.T) {
	epoch := operandLawEpoch(t)
	ascent := change.Set{Reasons: change.ChangedUnit, Direction: change.Known | change.Ascends}
	state := &epoch.regions[0]
	state.rememberAt = epoch.operands.advance()
	state.pending = change.Classified()
	if !epoch.markSourceOperands(0, ascent) {
		t.Fatal("marking the external source was refused")
	}
	if state.externalAt <= state.rememberAt {
		t.Fatal("an external environment source did not stamp the external row")
	}
	if state.backAt > state.rememberAt || state.pointsAt > state.rememberAt {
		t.Fatal("an external source stamped a row it does not feed")
	}
	if !state.pending.Admits() {
		t.Fatalf("classified ascent evidence did not reach the region: %+v", state.pending)
	}
	if !epoch.markSourceOperands(1, ascent) {
		t.Fatal("marking the head source was refused")
	}
	if state.backAt <= state.rememberAt || state.pointsAt <= state.rememberAt {
		t.Fatal("the head source did not stamp its back and interior rows")
	}
}

// TestRegionIngressReadersAgreeWithTheOperandTicks is the equivalence the
// deleted version vectors used to establish by an elementwise diff: a Region
// reports changed ingress exactly when one of its own operand ordinals
// carries a tick above its remember stamp.
func TestRegionIngressReadersAgreeWithTheOperandTicks(t *testing.T) {
	epoch := operandLawEpoch(t)
	state := &epoch.regions[0]
	state.hasExact = true
	scan := func(kinds ...operandKind) bool {
		for _, kind := range kinds {
			begin, end, ok := epoch.operands.plane.regionWindow(0, kind)
			if !ok {
				t.Fatalf("region window %d was refused", kind)
			}
			for ordinal := begin; ordinal < end; ordinal++ {
				if epoch.operands.changedSince(uint32(ordinal), state.rememberAt) {
					return true
				}
			}
		}
		return false
	}
	steps := []struct {
		mark int
		name string
	}{{-1, "quiescent"}, {0, "external source"}, {1, "head source"}}
	for _, step := range steps {
		state.rememberAt = epoch.operands.advance()
		state.pending = change.Set{}
		if step.mark >= 0 && !epoch.markSourceOperands(step.mark, change.Set{Direction: change.Known | change.Ascends}) {
			t.Fatalf("%s: mark refused", step.name)
		}
		external := scan(operandExternalProducer, operandExternalEnvironment, operandExternalFactor)
		if epoch.regionExternalIngressChanged(0) != external {
			t.Fatalf("%s: external reader disagrees with the operand ticks", step.name)
		}
		back := scan(operandBackProducer, operandBackEnvironment, operandBackFactor)
		if epoch.regionExactInputsChanged(0) != (external || back) {
			t.Fatalf("%s: exact-inputs reader disagrees with the operand ticks", step.name)
		}
	}
}

// TestRegionSnapshotReaderAgreesWithTheInteriorTicks holds the same
// equivalence for the WTO pass, which used to copy every interior version at
// EventEnter and diff it at EventExit.
func TestRegionSnapshotReaderAgreesWithTheInteriorTicks(t *testing.T) {
	epoch := operandLawEpoch(t)
	if !epoch.snapshotRegion(0) {
		t.Fatal("region snapshot was refused")
	}
	if epoch.regionSnapshotChanged(0) {
		t.Fatal("a quiescent pass reported interior movement")
	}
	if !epoch.markSourceOperands(0, change.Set{Direction: change.Known | change.Ascends}) {
		t.Fatal("marking a point outside the region was refused")
	}
	if epoch.regionSnapshotChanged(0) {
		t.Fatal("a publication outside the region was reported as interior movement")
	}
	if !epoch.markSourceOperands(1, change.Set{Direction: change.Known | change.Ascends}) {
		t.Fatal("marking the interior point was refused")
	}
	if !epoch.regionSnapshotChanged(0) {
		t.Fatal("an interior publication was not reported")
	}
}

// TestAccumulatorRefusesUnclassifiedAndDescendingOperands is the accumulator's
// admissibility law. The evidence axis is the whole predicate: no reason bit,
// no operand count and no phase can admit a reuse the direction refuses.
func TestAccumulatorRefusesUnclassifiedAndDescendingOperands(t *testing.T) {
	cases := []struct {
		name     string
		episode  regionEpoch
		admitted bool
	}{
		{"narrow phase", regionEpoch{phase: phaseNarrow, hasAccumulator: true, pending: change.Set{Direction: change.Known | change.Ascends}}, false},
		{"no accumulator", regionEpoch{phase: phaseAscent, pending: change.Set{Direction: change.Known | change.Ascends}}, false},
		{"unclassified operand", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Reasons: change.SupportAdded}}, false},
		{"descending operand", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Direction: change.Known | change.Descends}}, false},
		{"ascent mixed with a descent", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Direction: change.Known | change.Ascends}.Union(change.Set{Direction: change.Known | change.Descends})}, false},
		{"ascent mixed with an unclassified operand", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Direction: change.Known | change.Ascends}.Union(change.Set{})}, false},
		{"quiescent classified ascent", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Direction: change.Known}}, true},
		{"classified ascent", regionEpoch{phase: phaseAscent, hasAccumulator: true, pending: change.Set{Reasons: change.ChangedUnit, Direction: change.Known | change.Ascends}}, true},
	}
	for _, testCase := range cases {
		episode := testCase.episode
		if regionAccumulatorEvidenceAdmits(&episode) != testCase.admitted {
			t.Fatalf("%s: accumulator admissibility=%t want %t", testCase.name, !testCase.admitted, testCase.admitted)
		}
	}
}

// TestRegionContainsAgreesWithTheFrontierRegionPoints proves the runtime-local
// membership test is the inverse of the region point rows it is installed
// with. It never reads the base graph's region table, whose ordinals belong to
// a different frontier.
func TestRegionContainsAgreesWithTheFrontierRegionPoints(t *testing.T) {
	epoch := operandLawEpoch(t)
	// A second frontier's region ordinals exceed the base graph's table. The
	// law requires the two index spaces to differ: an implementation that
	// resolved membership through runtime.graph would read a region ordinal
	// the base graph does not have.
	epoch.runtime.regions = append(epoch.runtime.regions, runtimeRegion{active: true, head: 0, parent: schedule.NoRegion, points: []int{0, 1}})
	if len(epoch.runtime.regions) <= epoch.runtime.graph.RegionCount() {
		t.Fatalf("frontier regions=%d base graph regions=%d, want the frontier to exceed the base", len(epoch.runtime.regions), epoch.runtime.graph.RegionCount())
	}
	epoch.runtime.pointRegion = []int{1, 0}
	epoch.runtime.regions[0].parent = 1
	epoch.regions = append(epoch.regions, regionEpoch{})
	for region := range epoch.runtime.regions {
		members := map[int]bool{}
		for _, point := range epoch.runtime.regions[region].points {
			members[point] = true
		}
		for pointIndex := 0; pointIndex < epoch.runtime.graph.PointCount(); pointIndex++ {
			point, ok := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
			if !ok {
				t.Fatalf("point %d is not graph-issued", pointIndex)
			}
			inside, contained := epoch.regionContains(region, point)
			if !contained {
				t.Fatalf("region %d point %d membership was refused", region, pointIndex)
			}
			if inside != members[pointIndex] {
				t.Fatalf("region %d point %d inside=%t want %t", region, pointIndex, inside, members[pointIndex])
			}
		}
	}
}
