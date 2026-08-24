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

func returnEscapeTagAccessor() definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: returnEscapePackagePath,
		Name:        "Predicate",
		Receiver:    returnEscapeGoType("Route"),
		ResultIndex: -1,
	}
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
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
				},
				CandidateProvider: member.AxisRelationCandidate(provider),
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
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				// The route coordinate a member is published at, paired with
				// the destination below. A routed member carries its tag beside
				// its destination, so the selection that observes it is
				// correlated by this projection rather than by a second
				// addressing mode.
				Name:              "ReturnRouteTag",
				Key:               "placement/return-escape/route-tag",
				Relation:          "ReturnRoutes",
				Role:              member.Predicate,
				Result:            "ReturnRouteTagCarrier",
				Accessor:          returnEscapeTagAccessor(),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name:              "ReturnRouteDestination",
				Key:               "placement/return-escape/route-destination",
				Relation:          "ReturnRoutes",
				Role:              member.Destination,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor(1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "ReturnEscapeReducer",
			Key:  "placement/return-escape/reducer",
			// The carrying join declares ReturnRouteTag as its Predicate, so a
			// tag is required exactly as 5a441b2819 states: the selection is
			// correlated by the owner-issued projection this input names.
			Inputs: []definition.ReducerInput{{
				Axis:         placementAxis(),
				Carrier:      "PlacementFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Tag:          "ReturnRouteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: returnEscapePackagePath, Name: "ReturnEscapeFold", ResultIndex: 0},
		}},
	}
}
