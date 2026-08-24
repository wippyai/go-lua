package program

import (
	"testing"

	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// TestFormalFreezeProgramSealsARoutedWriteOverPrerequisiteJoins holds the
// freeze declaration to the two things a declaration package can decide on its
// own: that the Program is well formed, and that it agrees with the call shape
// of the reducer it names.
//
// The second is the one a Program cannot see by itself. Joins 0 and 1 are
// materializations join 2 depends on, not arguments; declaring them as fold
// inputs would be well formed in isolation and wrong against the owner, which
// takes exactly one selected Heap cell and its route coordinate.
func TestFormalFreezeProgramSealsARoutedWriteOverPrerequisiteJoins(t *testing.T) {
	declaration := FormalFreeze()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("freeze declaration rejected: %+v", problem)
	}
	reducer, reducerOK := heapdomain.AxisMemberCatalog().Reducer(heapdomain.FormalFreezeReducer)
	if !reducerOK {
		t.Fatal("generated Heap formal-freeze reducer unavailable")
	}
	if problem, valid := declaration.CheckAgainst(reducer); !valid {
		t.Fatalf("freeze declaration does not agree with its generated reducer call shape: %+v", problem)
	}

	if declaration.Candidate.AxisRelation.Member != MountedCallCandidates || declaration.Candidate.AxisRelation.Axis.Key != "call" {
		t.Fatalf("candidate=%+v, want the Call mounted-call directory", declaration.Candidate)
	}
	if got, want := declaration.JoinCount(), 3; got != want {
		t.Fatalf("join count=%d, want %d", got, want)
	}

	call, callOK := declaration.JoinAt(0)
	if !callOK || call.Read.Form != ruleprogram.Exact || call.Read.Axis.EntryReference().Key != "call" ||
		call.Relation.Member != MountedCallFacts || call.Key.Member != MountedCallFactKey ||
		len(call.Sources) != 1 || !call.Sources[0].Candidate || call.Predicate.Declared() {
		t.Fatalf("call join=%+v, want the candidate-keyed exact call fact read", call)
	}

	actuals, actualsOK := declaration.JoinAt(1)
	if !actualsOK || actuals.Read.Form != ruleprogram.Selected || actuals.Read.Axis.EntryReference().Key != "value" ||
		actuals.Relation.Member != FormalFreezeActualMembers || actuals.Key.Member != FormalFreezeActualKey ||
		actuals.Predicate.Member != FormalFreezeActualTag || len(actuals.Sources) != 1 || !actuals.Sources[0].Candidate {
		t.Fatalf("actual join=%+v, want the tagged selected member set", actuals)
	}
	if actuals.Read.Contract.Order != ruleprogram.OrderByTag || actuals.Read.Contract.Multiplicity != ruleprogram.MultiplicityOne ||
		actuals.Read.Contract.Sparse != ruleprogram.SparseDefault || !actuals.Read.Contract.DenominatorRef.Available() {
		t.Fatalf("actual read contract=%+v, want one defaulted cell per member ranked by owner tag", actuals.Read.Contract)
	}

	routes, routesOK := declaration.JoinAt(2)
	if !routesOK || routes.Read.Form != ruleprogram.Selected || routes.Read.Axis.EntryReference().Key != "heap" ||
		routes.Relation.Member != FormalFreezeRoutes || routes.Key.Member != FormalFreezeRouteKey ||
		routes.Predicate.Declared() || len(routes.Sources) != 3 {
		t.Fatalf("route join=%+v, want the dependent Heap route relation over candidate and both priors", routes)
	}
	if !routes.Sources[0].Candidate || routes.Sources[1].Candidate || routes.Sources[1].Position != 0 ||
		routes.Sources[2].Candidate || routes.Sources[2].Position != 1 {
		t.Fatalf("route sources=%+v, want candidate then join 0 then join 1", routes.Sources)
	}

	if len(declaration.Fold.Inputs) != 1 || declaration.Fold.Inputs[0] != 2 {
		t.Fatalf("fold inputs=%v, want only the route join - a prerequisite is not an argument", declaration.Fold.Inputs)
	}
	if len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("output count=%d, want 1", len(declaration.Fold.Outputs))
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 ||
		output.Destination.Member != FormalFreezeRouteDestination || output.ValueSlot != 0 {
		t.Fatalf("output=%+v, want a routed publication over join 2", output)
	}
	if declaration.Carry == nil || declaration.Carry.Mode != ruleprogram.CarryIdentity ||
		declaration.Carry.Input != 2 || declaration.Carry.Transform.Declared() {
		t.Fatalf("carry=%+v, want the identity carry a routed row keeps its unrouted world with", declaration.Carry)
	}
}
