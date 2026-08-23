package codegen

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	membergenerator "github.com/wippyai/go-lua/analysis/schema/axis/member/generator"
)

// routedProviderRoster is the fixture roster with the routed input declaring
// the coordinate it publishes at.
func routedProviderRoster(route memberdefinition.GoType) []membergenerator.Metadata {
	roster := []membergenerator.Metadata{providerConsumerMetadata(), providerDirectoryMetadata()}
	roster[0].Reducers[0].Inputs[0].Route = route
	return roster
}

// TestARoutedFoldReceivesTheCoordinateItPublishesAt states what the route
// carrier is: the Destination projection result of the join an output routes
// through, delivered as a value.
//
// A routed fold's answer may depend on which coordinate it is writing - a
// freeze judgment is indexed by the key it freezes - and before this the only
// way to learn it was to re-derive the route plan inside the fold, which is the
// declaration's statement, not the fold's. The coordinate is now a declared
// carrier like every other parameter, so the fold reads it and the plan stays
// where it is stated.
func TestARoutedFoldReceivesTheCoordinateItPublishesAt(t *testing.T) {
	catalog := providerPlanCatalog(t)
	model, err := Build(routedProviderRoster(providerType("example/placement", "Coordinate")), catalog)
	if err != nil || !model.Available() {
		t.Fatalf("routed reducer model rejected: err=%#v", err)
	}
	row, rowOK := model.At(0)
	if !rowOK {
		t.Fatal("routed rule missing")
	}
	input, inputOK := row.Reducer().Inputs[0], len(row.Reducer().Inputs) == 1
	if !inputOK || !input.Routed || input.Route != providerType("example/placement", "Coordinate") {
		t.Fatalf("routed input = %+v", input)
	}
	want := []ReducerArgument{
		{Role: ReducerArgumentRoute, Type: providerType("example/placement", "Coordinate"), Input: 0},
		{Role: ReducerArgumentTag, Type: providerType("example/placement", "Tag"), Input: 0},
		{Role: ReducerArgumentFact, Type: providerType("example/placement", "Fact"), Input: 0},
	}
	arguments := row.Reducer().Arguments()
	if len(arguments) != len(want) {
		t.Fatalf("argument count = %d, want %d", len(arguments), len(want))
	}
	for index, argument := range arguments {
		if argument != want[index] {
			t.Fatalf("argument %d = %+v, want %+v", index, argument, want[index])
		}
	}
}

// TestBuildRefusesARouteCarrierNoOutputDelivers is the fence on the carrier.
//
// A route coordinate exists because an output writes through that join; where
// no output does, there is no coordinate to hand the fold, and a declaration
// that names one is naming a value the call can never carry. The refusal is by
// the join the input reads, not by the input's own row, because routing is the
// Program's statement - which is also why declaring no route on a routed join
// is admitted: a fold whose answer does not depend on where it publishes does
// not grow a parameter for it, exactly as it does not grow one for a candidate
// it never reads.
func TestBuildRefusesARouteCarrierNoOutputDelivers(t *testing.T) {
	coordinate := providerType("example/placement", "Coordinate")
	spareCatalog := providerPlanCatalogFor(t, providerDeclaration(true))
	spareRoster := func() []membergenerator.Metadata {
		roster := []membergenerator.Metadata{providerConsumerMetadata(), providerDirectoryMetadata()}
		spare := roster[0].Reducers[0].Inputs[0]
		spare.Form, spare.Tag = member.Exact, memberdefinition.GoType{}
		roster[0].Reducers[0].Inputs = append(roster[0].Reducers[0].Inputs, spare)
		return roster
	}
	t.Run("unrouted-join", func(t *testing.T) {
		roster := spareRoster()
		roster[0].Reducers[0].Inputs[1].Route = coordinate
		if _, err := Build(roster, spareCatalog); err == nil {
			t.Fatal("route carrier admitted on an input no output routes through")
		}
	})
	t.Run("routed-join-admitted", func(t *testing.T) {
		roster := spareRoster()
		roster[0].Reducers[0].Inputs[0].Route = coordinate
		if _, err := Build(roster, spareCatalog); err != nil {
			t.Fatalf("route carrier refused on the join its output routes through: %#v", err)
		}
	})
	t.Run("no-route-declared", func(t *testing.T) {
		if _, err := Build(spareRoster(), spareCatalog); err != nil {
			t.Fatalf("routed join refused a fold that takes no coordinate: %#v", err)
		}
	})
	t.Run("wrong-carrier", func(t *testing.T) {
		if _, err := Build(routedProviderRoster(providerType("example/placement", "Tag")), providerPlanCatalog(t)); err == nil {
			t.Fatal("route carrier admitted against the wrong destination projection")
		}
	})
}

// TestASelectedInputLeavesItsTagToTheJoinThatDeclaresIt is the relaxation half.
//
// A tag is required exactly when the join declares a Predicate, and that is the
// reading Program's statement. The declaration used to demand a tag on every
// Selected input, which made a routed join with no predicate - the shape a
// freeze reads - undeclarable even though the model already checked it
// correctly. The declaration now follows the rule its own checker states.
func TestASelectedInputLeavesItsTagToTheJoinThatDeclaresIt(t *testing.T) {
	input := member.ReducerInput{
		Axis:         providerAxisRef(providerPlacementAxis),
		Carrier:      "carrier/placement/fact",
		Form:         member.Selected,
		Multiplicity: member.MultiplicityOne,
		Route:        "carrier/placement/key",
	}
	if !input.Available() {
		t.Fatal("an untagged routed selected input is not declarable")
	}
	if _, err := Build(routedProviderRoster(providerType("example/placement", "Coordinate")), providerPlanCatalog(t)); err != nil {
		t.Fatalf("a predicated join refused its tagged input: %#v", err)
	}
	roster := routedProviderRoster(providerType("example/placement", "Coordinate"))
	roster[0].Reducers[0].Inputs[0].Tag = memberdefinition.GoType{}
	if _, err := Build(roster, providerPlanCatalog(t)); err == nil {
		t.Fatal("a join declaring a predicate admitted an input with no tag")
	}
}
