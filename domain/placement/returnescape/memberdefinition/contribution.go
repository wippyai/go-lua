// Package memberdefinition is the generator-only contribution for Placement's
// return-escape reducer. It deliberately contains no runtime rule handle or
// route planner; the dependent route relation remains gated on the shared
// issuance seam.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const returnEscapePackagePath = "github.com/wippyai/go-lua/domain/placement/returnescape"

const valuePackagePath = "github.com/wippyai/go-lua/domain/value"

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

func valueAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

// returnBoundaryCandidateProvider is Value's owner-issued return-boundary
// candidate directory. ReturnEscape's route relation is a dependent relation
// over this foreign candidate, exactly as its Program's Candidate names it.
func returnBoundaryCandidateProvider() member.RelationRef {
	return member.RelationRef{Axis: valueAxis(), Member: "value/return-boundary/candidates"}
}

func returnEscapeGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: returnEscapePackagePath, Name: name}
}

func returnEscapeSymbol(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: returnEscapePackagePath, Name: name, ResultIndex: 0}
}

func routeAccessor(resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: returnEscapePackagePath,
		Name:        "Coordinates",
		Receiver:    returnEscapeGoType("Route"),
		ResultIndex: resultIndex,
	}
}

// Contribution declares Placement's return-escape judgment: the dependent
// ReturnRoutes relation Value's own boundary candidate makes possible, and
// the one reducer that fold receives its authenticated selected predecessor
// through.
func Contribution() definition.Contribution {
	provider := returnBoundaryCandidateProvider()
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-return-escape",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/heap", Name: "Key"}},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Fact"}},
			{Name: "ReturnRouteTagCarrier", Key: "carrier/placement/return-route-tag", Type: definition.GoType{Name: "uint64"}},
			{Name: "ReturnRouteCarrier", Key: "carrier/placement/return-route", Type: returnEscapeGoType("Route")},
			{Name: "ReturnBoundaryCarrier", Key: "carrier/value/return-boundary", Type: definition.GoType{PackagePath: valuePackagePath, Name: "ReturnBoundary"}},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: definition.GoType{PackagePath: valuePackagePath, Name: "Value"}},
		},
		Relations: []definition.Relation{
			{
				Name:    "ReturnRoutes",
				Key:     "placement/return-escape/routes",
				Subject: "ReturnRouteCarrier",
				// Candidate, the exact root read, then the delivered Value
				// member vector, in the Program's own join order. The root is
				// an authenticated candidate prerequisite the route algebra
				// does not fold into its own value, exactly as Store's
				// StorageFold declares an unused source input, but it must
				// still be a reachable join.
				Inputs: []definition.RelationInput{
					{Carrier: "ReturnBoundaryCarrier"},
					{Carrier: "ValueFactCarrier"},
					{Carrier: "ValueFactCarrier", Many: true},
				},
				CandidateProvider: provider,
				Derivation: definition.RelationDerivation{
					State: returnEscapeGoType("RoutePlan"),
					Build: returnEscapeSymbol("DeriveReturnRoutes"),
					Count: returnEscapeSymbol("ReturnRouteCount"),
					At:    returnEscapeSymbol("ReturnRouteAt"),
					StaticAxes: []schema.EntryReference{
						placementAxis(),
						valueAxis(),
					},
				},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "ReturnRouteKey",
				Key:               "placement/return-escape/route-key",
				Relation:          "ReturnRoutes",
				Role:              member.Key,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor(0),
				CandidateProvider: provider,
			},
			{
				Name:              "ReturnRouteDestination",
				Key:               "placement/return-escape/route-destination",
				Relation:          "ReturnRoutes",
				Role:              member.Destination,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor(1),
				CandidateProvider: provider,
			},
		},
		Reducers: []definition.Reducer{{
			Name: "ReturnEscapeReducer",
			Key:  "placement/return-escape/reducer",
			// This input's carrying join declares no Predicate (5a441b2819: a
			// tag is required exactly when the join declares one), so the tag
			// is not a schema-checked selection Tag; it is transport from the
			// routed join's own RouteMember pair, declared here as Route.
			Inputs: []definition.ReducerInput{{
				Axis:         placementAxis(),
				Carrier:      "PlacementFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Route:        "ReturnRouteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: returnEscapePackagePath, Name: "ReturnEscapeFold", ResultIndex: 0},
		}},
	}
}
