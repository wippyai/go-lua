package linkexecutionplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// TestPlanAvailabilityIsSealedAtConstruction pins that a plan's availability is
// the verdict New reached over the lifted state edges, not a re-validation the
// accessors repeat.  Every accessor guard therefore reads one settled fact, and
// an unconstructed plan stays unavailable.
func TestPlanAvailabilityIsSealedAtConstruction(t *testing.T) {
	graph, _ := realPlanLawGraph(t)
	directory := newPlanLawDirectory(t, "same", "same")
	mounted, mountedOK := contextfiber.Mounted(directory.modules[0])
	global, globalOK := contextfiber.LinkGlobal(directory.directory.LinkID())
	if !mountedOK || !globalOK {
		t.Fatal("owners")
	}
	owners := []contextfiber.PointOwner{mounted, global}
	layout := newPlanLawLayout(t, graph, directory.directory, owners)
	plan, planOK := New(graph, layout, directory.directory, nil)
	if planOK != plan.Available() {
		t.Fatalf("plan verdict constructor=%v available=%v", planOK, plan.Available())
	}
	if !planOK || plan.StateEdgeCount() == 0 {
		t.Fatalf("sealed plan ok=%v edges=%d", planOK, plan.StateEdgeCount())
	}
	if plan.Available() != plan.Available() {
		t.Fatal("plan verdict is not stable")
	}
	if plan.StateCount() != layout.StateCount() || plan.PointCount() != layout.PointCount() {
		t.Fatal("sealed plan lost its layout projection")
	}

	var absent *LinkExecutionPlan
	if absent.Available() || (&LinkExecutionPlan{}).Available() {
		t.Fatal("unconstructed plan available")
	}
	if (&LinkExecutionPlan{}).StateCount() != 0 || (&LinkExecutionPlan{}).Layout().Available() {
		t.Fatal("unconstructed plan published a projection")
	}
}

// TestBoundEdgeAvailabilityIsSealedAtConstruction pins that NewBoundEdge is the
// sole authenticator: the accessors read the verdict it sealed rather than
// re-authenticating the graph, layout, and directory on every call.  Detaching
// the authorities from an already-issued edge therefore cannot flip the verdict,
// the read allocates nothing, and an unissued edge stays unavailable.
func TestBoundEdgeAvailabilityIsSealedAtConstruction(t *testing.T) {
	fixture := newBoundEdgeSCCFixture(t)
	edge := fixture.forward
	if !edge.Available() {
		t.Fatal("issued edge unavailable")
	}
	detached := edge
	detached.graph = nil
	detached.layout = contextfiber.Layout{}
	detached.directory = executioncontext.Directory{}
	if !detached.Available() {
		t.Fatal("Available re-authenticates the issuing authorities instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = edge.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (BoundEdge{}).Available() {
		t.Fatal("unissued edge available")
	}
	if (BoundEdge{}).From() != 0 || (BoundEdge{}).SourcePoint().Available() {
		t.Fatal("unissued edge published a projection")
	}
}

// TestBoundEdgeRefusesMalformedConstruction pins that every authentication the
// accessors once repeated is decided once, at issuance: a foreign graph, an
// unavailable layout, and a transition that is not the directory's canonical
// row are all refused by NewBoundEdge, which never issues an unavailable edge.
func TestBoundEdgeRefusesMalformedConstruction(t *testing.T) {
	fixture := newBoundEdgeSCCFixture(t)
	if edge, ok := NewBoundEdge(nil, fixture.layout, fixture.directory, fixture.points[0], fixture.points[1], boundEdgeSpec(t, fixture.directory, fixture.forwardRow, fixture.forwardGen)); ok || edge.Available() {
		t.Fatal("graphless edge issued")
	}
	if edge, ok := NewBoundEdge(fixture.graph, contextfiber.Layout{}, fixture.directory, fixture.points[0], fixture.points[1], boundEdgeSpec(t, fixture.directory, fixture.forwardRow, fixture.forwardGen)); ok || edge.Available() {
		t.Fatal("layoutless edge issued")
	}
	if edge, ok := NewBoundEdge(fixture.graph, fixture.layout, executioncontext.Directory{}, fixture.points[0], fixture.points[1], boundEdgeSpec(t, fixture.directory, fixture.forwardRow, fixture.forwardGen)); ok || edge.Available() {
		t.Fatal("directoryless edge issued")
	}
	if edge, ok := NewBoundEdge(fixture.graph, fixture.layout, fixture.directory, fixture.points[0], fixture.points[1], boundEdgeSpec(t, fixture.directory, fixture.reverseRow, fixture.forwardGen)); ok || edge.Available() {
		t.Fatal("edge issued for a transition that does not join its generation")
	}
	if edge, ok := NewBoundEdge(fixture.graph, fixture.layout, fixture.directory, fixture.points[1], fixture.points[0], boundEdgeSpec(t, fixture.directory, fixture.forwardRow, fixture.forwardGen)); ok || edge.Available() {
		t.Fatal("edge issued for endpoints that contradict the transition owners")
	}
}
