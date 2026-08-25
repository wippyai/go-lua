package program

import (
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"testing"
)

func TestPublicationEscapeProgramKeepsEffectCallValueAndPlacementAuthoritiesSeparate(t *testing.T) {
	declaration := PublicationEscape()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("publication declaration rejected: %+v", problem)
	}
	if declaration.Candidate.AxisRelation.Axis.Key != "effect" || declaration.Candidate.AxisRelation.Member != EffectMountedCallCandidates || declaration.JoinCount() != 3 {
		t.Fatalf("candidate=%+v joins=%d", declaration.Candidate, declaration.JoinCount())
	}
	callJoin, _ := declaration.JoinAt(0)
	valueJoin, _ := declaration.JoinAt(1)
	routeJoin, _ := declaration.JoinAt(2)
	if callJoin.Read.Form != ruleprogram.Exact || valueJoin.Read.Form != ruleprogram.Selected || routeJoin.Read.Form != ruleprogram.Selected {
		t.Fatalf("forms=%v/%v/%v", callJoin.Read.Form, valueJoin.Read.Form, routeJoin.Read.Form)
	}
	if valueJoin.Read.Axis.EntryReference().Key != "value" || routeJoin.Read.Axis.EntryReference().Key != "placement" {
		t.Fatalf("axes=%v/%v", valueJoin.Read.Axis, routeJoin.Read.Axis)
	}
	if valueJoin.Relation.Axis.Key != "effect" || valueJoin.Key.Axis.Key != "effect" || valueJoin.Predicate.Axis.Key != "effect" {
		t.Fatalf("source relation axes=%v/%v/%v, want Effect-owned descriptors read through Value", valueJoin.Relation.Axis, valueJoin.Key.Axis, valueJoin.Predicate.Axis)
	}
	if callJoin.Read.PointBound != ruleprogram.PointBound || valueJoin.Read.PointBound != ruleprogram.PointBound || routeJoin.Read.PointBound != ruleprogram.PointBound {
		t.Fatalf("point bounds=%v/%v/%v, want the transported operand point", callJoin.Read.PointBound, valueJoin.Read.PointBound, routeJoin.Read.PointBound)
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Destination.Member != PublicationRouteDestination {
		t.Fatalf("output=%+v", output)
	}
}
