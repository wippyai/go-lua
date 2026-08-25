// Package memberdefinition is the generator-only owner source for the
// Placement formal rule's own route relation, its projections, and its
// reducer. It is imported by the member definition roster and by nothing at
// runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	formalPackagePath    = "github.com/wippyai/go-lua/domain/placement/formal"
	placementPackagePath = "github.com/wippyai/go-lua/domain/placement"
	callPackagePath      = "github.com/wippyai/go-lua/domain/call"
	heapPackagePath      = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func placementAxis() schema.EntryReference { return axisReference("placement") }

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func formalFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: formalPackagePath, Name: name, ResultIndex: 0}
}

func routeAccessor(name string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: formalPackagePath,
		Name:        name,
		Receiver:    goType(formalPackagePath, "Route"),
		ResultIndex: resultIndex,
	}
}

// mountedCallProvider is the foreign candidate directory every row this rule
// declares hangs off: Call owns which mounted calls exist, and Placement
// mirrors that directory nowhere.
func mountedCallProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}
}

// Contribution is the Placement formal rule's own member declaration.
//
// The rule reads three things and folds one. Its candidate is a mounted call.
// Join zero is that call's own fact. Join one is the call's ordered mounted
// actuals, a nested member set Value publishes. Join two is the formal route
// set derived from those two, and it is the only row here whose construction
// is still authored.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-formal",
		Carriers: []definition.Carrier{
			// Placement's own two carriers, repeated verbatim from the axis
			// declaration so this rule's fold and projection shapes are
			// derivable from the contribution alone; composition refuses a
			// repeat that disagrees.
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact")},
			// The two Call carriers this rule's route relation is typed in.
			// They repeat Call's own declaration verbatim; composition refuses
			// a repeat that disagrees.
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: goType(callPackagePath, "CallCoordinate")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "FormalRouteCarrier", Key: "carrier/placement/formal-route", Type: goType(formalPackagePath, "Route")},
			{Name: "FormalRouteTagCarrier", Key: "carrier/placement/formal-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
			{
				// The formal route set: the exact allocation roots the
				// formal ownership rows of this call's known targets demand,
				// each under the escape its row authored. Its inputs are the
				// candidate, the call fact, and the whole vector of the
				// call's actuals - a demand reduced over every formal
				// selector range cannot be handed one actual at a time.
				Name:    "FormalRoutes",
				Key:     "placement/formal/routes",
				Subject: "FormalRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "CallCoordinateCarrier"},
					{Carrier: "CallFactCarrier"},
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
				},
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Derivation: definition.RelationDerivation{
					State: goType(formalPackagePath, "RoutePlan"),
					Build: formalFunction("DeriveFormalRoutes"),
					Count: formalFunction("FormalRouteCount"),
					At:    formalFunction("FormalRouteAt"),
					StaticAxes: []schema.EntryReference{
						placementAxis(),
						axisReference("value"),
						axisReference("call"),
						axisReference("pack"),
					},
				},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "FormalRouteKey",
				Key:               "placement/formal/route-key",
				Relation:          "FormalRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Key,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor("Coordinates", 0),
			},
			{
				// The route coordinate a member is published at, carrying
				// the escape the formal row authored, or the unknown code
				// where the declaration authenticated a widening. The routed
				// worker pairs a cell with its member by this tag, and the
				// fold reads the policy out of the same tag rather than
				// deriving a second one.
				Name:              "FormalRouteTag",
				Key:               "placement/formal/route-tag",
				Relation:          "FormalRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Predicate,
				Result:            "FormalRouteTagCarrier",
				Accessor:          routeAccessor("Predicate", -1),
			},
			{
				// A formal row displaces the world at the very root it read,
				// so the destination is the same coordinate under its own
				// role.
				Name:              "FormalRouteDestination",
				Key:               "placement/formal/route-destination",
				Relation:          "FormalRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Destination,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor("Coordinates", 1),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "FormalReducer",
			Key:  "placement/formal/reducer",
			Inputs: []definition.ReducerInput{{
				Axis:         placementAxis(),
				Carrier:      "PlacementFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Tag:          "FormalRouteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: formalPackagePath, Name: "FormalFold", ResultIndex: 0},
		}},
	}
}
