// Package memberdefinition declares Call dispatch's generated relation and
// reducer vocabulary. It is generator-only; runtime code consumes the sealed
// catalog and direct generated calls.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	callPackagePath  = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath = "github.com/wippyai/go-lua/domain/value"
	routePackagePath = "github.com/wippyai/go-lua/domain/call/dispatch/route"
)

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func routeFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: routePackagePath, Name: name, ResultIndex: 0}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: callPackagePath, Name: name, Receiver: goType(callPackagePath, "DispatchRoute"), ResultIndex: result}
}

func mountedCallProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}
}

// Contribution is the whole Call-owned dispatch relation. Value and Heap are
// static derivation inputs only; the relation publishes a Call route and the
// reducer receives only Call carriers plus the owner-issued uint64 tag.
func Contribution() definition.Contribution {
	provider := member.AxisRelationCandidate(mountedCallProvider())
	return definition.Contribution{
		Axis: "call",
		Rule: "call-dispatch",
		Carriers: []definition.Carrier{
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
			{Name: "DispatchRouteCarrier", Key: "carrier/call/dispatch-route", Type: goType(callPackagePath, "DispatchRoute")},
			{Name: "DispatchRouteTagCarrier", Key: "carrier/call/dispatch-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{{
			Name:              "DispatchRoutes",
			Key:               "call/dispatch/routes",
			Subject:           "DispatchRouteCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}, {Carrier: "ValueFactCarrier"}},
			CandidateProvider: provider,
			Derivation: definition.RelationDerivation{
				State: goType(routePackagePath, "Plan"),
				Build: routeFunction("Derive"),
				Count: routeFunction("Count"),
				At:    routeFunction("At"),
				StaticAxes: []schema.EntryReference{
					axisReference("call"),
					axisReference("value"),
					axisReference("heap"),
				},
			},
		}},
		Projections: []definition.Projection{
			{Name: "DispatchRouteKey", Key: "call/dispatch/route-key", Relation: "DispatchRoutes", CandidateProvider: provider, Role: member.Key, Result: "CallKeyCarrier", Accessor: routeMethod("Coordinates", 0)},
			{Name: "DispatchRouteTag", Key: "call/dispatch/route-tag", Relation: "DispatchRoutes", CandidateProvider: provider, Role: member.Predicate, Result: "DispatchRouteTagCarrier", Accessor: routeMethod("Predicate", -1)},
			{Name: "DispatchRouteDestination", Key: "call/dispatch/route-destination", Relation: "DispatchRoutes", CandidateProvider: provider, Role: member.Destination, Result: "CallKeyCarrier", Accessor: routeMethod("Coordinates", 1)},
		},
		Reducers: []definition.Reducer{{
			Name:      "DispatchReducer",
			Key:       "call/dispatch/reducer",
			Candidate: "CallCoordinateCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:         axisReference("call"),
				Carrier:      "CallFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Tag:          "DispatchRouteTagCarrier",
				Route:        "CallKeyCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: axisReference("call"), Carrier: "CallFactCarrier"}},
			Implementation: routeFunction("Fold"),
		}},
	}
}
