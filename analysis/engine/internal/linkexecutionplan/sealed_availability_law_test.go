package linkexecutionplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
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
