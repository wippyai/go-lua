package engine

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// statePlaneLawFixture is one real mounted Link plan whose context cardinality
// is a parameter, so a state-plane builder's cost can be pinned against the
// exact plane it addresses rather than against a wall clock.
type statePlaneLawFixture struct {
	graph    *equation.Graph
	layout   contextfiber.Layout
	plan     *linkexecutionplan.LinkExecutionPlan
	contexts int
}

func newStatePlaneLawFixture(t *testing.T, contexts int) statePlaneLawFixture {
	t.Helper()
	if contexts <= 0 {
		t.Fatalf("state plane fixture contexts=%d", contexts)
	}
	graph, _, _ := newRegionDischargeGraph(t)
	if graph == nil || graph.PointCount() != 2 {
		t.Fatalf("state plane fixture graph points=%v", graph)
	}
	link := stateRegionLawID(t, "state-plane-link")
	module := stateRegionLawID(t, "state-plane-module")
	rows := make([]executioncontext.Context, 0, contexts)
	roots := make([]executioncontext.RootContext, 0, contexts)
	for index := 0; index < contexts; index++ {
		label := fmt.Sprintf("state-plane-actor-%d", index)
		row, rowOK := executioncontext.NewContext(link, module, stateRegionLawID(t, label), stateRegionLawID(t, label+"-representative"))
		root, rootOK := executioncontext.NewRootContext(link, stateRegionLawID(t, label+"-root"), row.ID())
		if !rowOK || !rootOK {
			t.Fatalf("state plane fixture context %d row=%v root=%v", index, rowOK, rootOK)
		}
		rows = append(rows, row)
		roots = append(roots, root)
	}
	directory, directoryOK := executioncontext.Seal(link, rows, roots, nil)
	if !directoryOK {
		t.Fatal("state plane fixture directory")
	}
	owner, ownerOK := contextfiber.Mounted(module)
	if !ownerOK {
		t.Fatal("state plane fixture owner")
	}
	owners := []contextfiber.PointOwner{owner, owner}
	index, indexOK := contextfiber.New(directory, len(owners), identity.Generation(1))
	if !indexOK {
		t.Fatal("state plane fixture index")
	}
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, owners, identity.Generation(1), graph)
	if !layoutOK {
		t.Fatal("state plane fixture layout")
	}
	plan, planOK := linkexecutionplan.New(graph, layout, directory, nil)
	if !planOK || plan == nil || int(plan.StateCount()) != contexts*len(owners) {
		t.Fatalf("state plane fixture plan ok=%v states=%d", planOK, plan.StateCount())
	}
	return statePlaneLawFixture{graph: graph, layout: layout, plan: plan, contexts: contexts}
}

// countingStatePlan counts the exact plane accesses a state-plane builder
// performs. It is the structural instrument the complexity law is stated over.
type countingStatePlan struct {
	plan       *linkexecutionplan.LinkExecutionPlan
	stateCount int
	stateAt    int
	lookup     int
}

func (counting *countingStatePlan) StateCount() contextfiber.StateOrdinal {
	counting.stateCount++
	return counting.plan.StateCount()
}

func (counting *countingStatePlan) StateAt(state contextfiber.StateOrdinal) (contextfiber.StateCell, bool) {
	counting.stateAt++
	return counting.plan.StateAt(state)
}

func (counting *countingStatePlan) Lookup(context contextfiber.ContextOrdinal, point contextfiber.PointOrdinal) (contextfiber.StateOrdinal, bool) {
	counting.lookup++
	return counting.plan.Lookup(context, point)
}

func statePlaneLawFactors(t *testing.T, count int) []runtimeFactorEdge {
	t.Helper()
	factors := make([]runtimeFactorEdge, count)
	for index := range factors {
		factors[index] = runtimeFactorEdge{index: index, key: regionLawKey(byte(index) + 11), source: 0, target: 1}
		if !factors[index].key.Available() {
			t.Fatalf("state plane factor key %d", index)
		}
	}
	return factors
}

// TestBuildStateFactorIndexCostIsLinearInThePlaneItAddresses pins the state
// plane's admission cost.  Plane cardinality is a settled fact of the sealed
// plan, so it is read a bounded number of times and never once per row; and
// resolving one factor endpoint's point owner is a bounded lookup rather than a
// rescan of the compact rows.  Either defect makes assembly quadratic in
// StateCount.
func TestBuildStateFactorIndexCostIsLinearInThePlaneItAddresses(t *testing.T) {
	fixture := newStatePlaneLawFixture(t, 8)
	factors := statePlaneLawFactors(t, 4)
	counting := &countingStatePlan{plan: fixture.plan}
	incoming, outgoing, rows, stateRows, ok := buildStateFactorIndex(fixture.graph, counting, factors, true)
	if !ok || len(incoming) != int(fixture.plan.StateCount()) || len(outgoing) != int(fixture.plan.StateCount()) || len(rows) == 0 || len(stateRows) != fixture.graph.PointCount() {
		t.Fatalf("state factor index ok=%v incoming=%d outgoing=%d rows=%d stateRows=%d", ok, len(incoming), len(outgoing), len(rows), len(stateRows))
	}
	stateCount := int(fixture.plan.StateCount())
	if counting.stateCount > 8 {
		t.Fatalf("state factor index read plane cardinality %d times for %d states and %d factors, bound 8", counting.stateCount, stateCount, len(factors))
	}
	// One inverse pass over the plane, plus a bounded owner resolution and one
	// contextual row per source occurrence for every factor edge.
	bound := stateCount + len(factors)*(fixture.contexts+4)
	if counting.stateAt > bound {
		t.Fatalf("state factor index performed %d cell reads for %d states and %d factors, bound %d", counting.stateAt, stateCount, len(factors), bound)
	}
}

// TestStateRowOwnerMatchesLayoutPointOwner pins the owner-resolution law the
// compact rows must satisfy: the owner read from a point's admitted state rows
// is exactly the layout's authenticated point owner, and an unmaterialized
// point is refused rather than defaulted.
func TestStateRowOwnerMatchesLayoutPointOwner(t *testing.T) {
	fixture := newStatePlaneLawFixture(t, 3)
	stateRows, rowsOK := buildStatePointRows(fixture.graph, fixture.plan, true)
	if !rowsOK || len(stateRows) != fixture.graph.PointCount() {
		t.Fatalf("state point rows ok=%v rows=%d", rowsOK, len(stateRows))
	}
	for point := range stateRows {
		expected, expectedOK := fixture.layout.PointOwnerAt(contextfiber.PointOrdinal(point))
		owner, ownerOK := stateRowOwner(fixture.plan, stateRows[point])
		if !expectedOK || !ownerOK || owner != expected {
			t.Fatalf("point %d owner expected=%v/%v derived=%v/%v", point, expected, expectedOK, owner, ownerOK)
		}
		if !owner.Available() || !owner.Mounted() {
			t.Fatalf("point %d owner lost authentication", point)
		}
	}
	if owner, ownerOK := stateRowOwner(fixture.plan, nil); ownerOK || owner.Available() {
		t.Fatal("unmaterialized point admitted an owner")
	}
}
