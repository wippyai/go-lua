package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// TestDispatchReadsItsCalleeAndPublishesAtItsOwnCoordinate states the whole
// declaration: one exact read of Value's image of the callee, and an exact
// publication at the mounted application's own Call coordinate.
//
// The absence of a Call-axis join is the point. A rule that selected the axis
// it publishes into would derive its authored region from its own output, and
// such a rule is not a monotone operator: its region can shrink between ascent
// steps and the fixpoint refuses it rather than converging.
func TestDispatchReadsItsCalleeAndPublishesAtItsOwnCoordinate(t *testing.T) {
	declaration := Dispatch()
	if problem, ok := declaration.Check(); !ok {
		t.Fatalf("dispatch declaration refused: %+v", problem)
	}
	if len(declaration.Joins) != 1 {
		t.Fatalf("dispatch declares %d joins, want the one callee read", len(declaration.Joins))
	}
	callee := declaration.Joins[0]
	if callee.Relation.Member != MountedCallParents || callee.Key.Member != MountedCallCalleeKey {
		t.Fatalf("dispatch callee join = %+v", callee)
	}
	if callee.Read.Form != ruleprogram.Exact || callee.Read.Contract.Multiplicity != ruleprogram.MultiplicityOne ||
		callee.Predicate.Member != "" || callee.Read.Contract.DenominatorRef.Key != "" {
		t.Fatalf("dispatch callee read = %+v", callee.Read)
	}
	if callee.Relation.Axis.Key == AxisKey {
		t.Fatalf("dispatch joins the axis it writes: %+v", callee.Relation)
	}
	fold := declaration.Fold
	if fold.Reducer != (member.ReducerRef{Axis: declaration.Candidate.AxisRelation.Axis, Member: DispatchReducer}) ||
		len(fold.Inputs) != 1 || fold.Inputs[0] != 0 || len(fold.Outputs) != 1 {
		t.Fatalf("dispatch fold = %+v", fold)
	}
	output := fold.Outputs[0]
	if output.Mode != ruleprogram.ModeExact || output.Destination.Member != MountedCallCoordinate || output.RouteJoinPresent {
		t.Fatalf("dispatch output = %+v", output)
	}
	if declaration.Carry != nil {
		t.Fatalf("dispatch carries a predecessor world: %+v", declaration.Carry)
	}
}
