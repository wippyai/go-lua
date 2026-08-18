package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TypeConformance is a call-argument assignment site. Result geometry must
// keep it off the branch plane: that plane is the Value-observation carrier
// for guard polarity, and a conformance row has no branch producers.
func mustResultGeometry(t *testing.T, state *compiledState) resultGeometry {
	t.Helper()
	if state == nil {
		t.Fatal("compiled state")
	}
	geometry, ok := state.resultGeometry()
	if !ok {
		t.Fatal("result geometry")
	}
	return geometry
}

func TestResultGeometryKeepsTypeConformanceOffTheBranchPlane(t *testing.T) {
	plan, status := Compile(fixtureLink(t, "advice/shape-polymorphic"))
	if status != CompileComplete || plan == nil || plan.state == nil {
		t.Fatalf("compile advice/shape-polymorphic = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	geometry := mustResultGeometry(t, plan.state)
	conformance := 0
	for _, observation := range geometry.branchObservations {
		if observation.kind == structure.DiagnosticObservationTypeConformance {
			t.Fatal("type-conformance occupied the branch observation plane")
		}
		if observation.kind != structure.DiagnosticObservationBranchCondition {
			t.Fatalf("branch plane held kind %v", observation.kind)
		}
	}
	for _, observation := range geometry.staticObservations {
		if observation.kind != structure.DiagnosticObservationTypeConformance {
			continue
		}
		if !observation.compiledTypeConformance.available() {
			t.Fatal("static type-conformance has no call-argument payload")
		}
		conformance++
	}
	if conformance == 0 {
		t.Fatal("advice/shape-polymorphic compiled no type-conformance observation")
	}
}
