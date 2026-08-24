package programschema_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

func TestRuleOccurrenceKeepsRouteProofSeparateFromDataInput(t *testing.T) {
	point, input, landing, routeID := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}
	route := programschema.RuleOccurrenceRoute{Point: landing, ID: routeID}
	row, ok := programschema.NewRuleOccurrenceWithInputs(
		schema.Key("rule/route-law"), schema.Key("axis/route-law"), 0,
		point, []identity.ContentID{input}, programissuance.StagePredecessor,
		programissuance.InputPreviousStage, route, false, programschema.RuleOccurrenceSource{},
	)
	if !ok {
		t.Fatal("rule occurrence rejected an independently-authored route proof")
	}
	gotInput, inputOK := row.InputPointAt(0)
	gotRoute, routeOK := row.PredecessorRoute()
	if !inputOK || gotInput != input || !routeOK || gotRoute != route || gotRoute.Point == gotInput {
		t.Fatalf("route proof and data input were conflated: input=%v/%t route=%+v/%t", gotInput, inputOK, gotRoute, routeOK)
	}
}

func TestRuleOccurrenceRefusesPartialRouteProof(t *testing.T) {
	point, input, landing, routeID := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}
	for name, route := range map[string]programschema.RuleOccurrenceRoute{
		"point-only": {Point: landing},
		"id-only":    {ID: routeID},
	} {
		if _, ok := programschema.NewRuleOccurrenceWithInputs(
			schema.Key("rule/route-law"), schema.Key("axis/route-law"), 0,
			point, []identity.ContentID{input}, programissuance.StagePredecessor,
			programissuance.InputPreviousStage, route, false, programschema.RuleOccurrenceSource{},
		); ok {
			t.Fatalf("%s route proof was admitted", name)
		}
	}
}
