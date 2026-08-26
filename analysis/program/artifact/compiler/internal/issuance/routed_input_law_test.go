package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

// routedRequest is one request for a stage that stands on a route.
func routedRequest(t *testing.T, base, route identity.ContentID) (schemaissuance.Plan, []Request) {
	t.Helper()
	table := scheduleTable(t)
	plan, ok := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/binary-presence-refinement",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormRoutePredecessor,
		Rule:        "rule/refine",
		Writes:      "axis/write",
	}})
	if !ok {
		t.Fatal("execution plan refused sealed subscription")
	}
	subscription, subscriptionOK := plan.At(0)
	if !subscriptionOK {
		t.Fatal("sealed subscription unavailable")
	}
	stage := scheduleEntry(t, table, programissuance.StageRoutePredecessor, schemaissuance.KindStage)
	routeInput := scheduleEntry(t, table, programissuance.InputRouteArrival, schemaissuance.KindInput)
	pointOne := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityOne}
	return plan, []Request{{
		subscription: subscription, stage: stage, base: base,
		parameters: []value{
			{typ: pointOne, present: true, points: []identity.ContentID{base}},
			{typ: schemaissuance.IdentityType(schemaissuance.TypeAxisKey), present: true, key: "axis/write"},
			{typ: schemaissuance.IdentityType(schemaissuance.TypeRouteIdentity), present: true, identity: route},
		},
		inputs: []Input{{declaration: routeInput}},
	}}
}

// TestRoutedInputRefusesWhenTheRouteHasNoSource states that the route-to-source
// mapping is total over the routes a declaration uses, or the schedule refuses.
//
// A routed stage reads the state its route delivers. If the host cannot say
// where that route comes from, there is no such state, and the one available
// substitute - this stage's position in its point's chain - is precisely the
// placement a routed stage exists to leave. Falling back to it would restore
// the defect while reporting success, so the absence is refused instead.
func TestRoutedInputRefusesWhenTheRouteHasNoSource(t *testing.T) {
	base, route := testID(1), testID(7)
	plan, requests := routedRequest(t, base, route)
	if _, scheduled := BuildSchedule(41, plan, requests, func(identity.ContentID) (identity.ContentID, bool) {
		return identity.ContentID{}, false
	}); scheduled {
		t.Fatal("a routed stage was scheduled against a route the host could not place")
	}
	if _, scheduled := BuildSchedule(41, plan, requests, nil); scheduled {
		t.Fatal("a routed stage was scheduled with no route resolver at all")
	}
}

// TestRoutedInputReadsTheStateItsRouteDelivers is the positive half: given a
// resolver that places the route, the stage's input is that route's source and
// not its own base.
func TestRoutedInputReadsTheStateItsRouteDelivers(t *testing.T) {
	base, route, source := testID(1), testID(7), testID(9)
	plan, requests := routedRequest(t, base, route)
	schedule, scheduled := BuildSchedule(41, plan, requests, func(candidate identity.ContentID) (identity.ContentID, bool) {
		return source, candidate == route
	})
	if !scheduled {
		t.Fatal("a routed stage with a placed route was refused")
	}
	emission, emissionOK := schedule.EmissionAt(schedule.EmissionCount() - 1)
	if !emissionOK {
		t.Fatal("routed emission unavailable")
	}
	input, inputOK := emission.InputPointAt(0)
	if !inputOK || input != source {
		t.Fatalf("routed input = %v, want the route's source %v", input, source)
	}
}
