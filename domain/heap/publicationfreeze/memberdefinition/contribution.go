// Package memberdefinition is the generator-only owner source for the Heap
// publication-freeze rule's own relations, projections and reducer. It is
// imported by the member definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	effectFactorPackagePath = "github.com/wippyai/go-lua/domain/effect/factor"
	heapPackagePath         = "github.com/wippyai/go-lua/domain/heap"
	callPackagePath         = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath        = "github.com/wippyai/go-lua/domain/value"
	freezePackagePath       = "github.com/wippyai/go-lua/domain/heap/publicationfreeze"
	recentPlanPackagePath   = "github.com/wippyai/go-lua/domain/heap/internal/recentplan"
)

func heapAxis() schema.EntryReference { return axisReference("heap") }

func freezeFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: freezePackagePath, Name: name, ResultIndex: 0}
}

func routeMethod(name string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: recentPlanPackagePath,
		Name:        name,
		Receiver:    goType(recentPlanPackagePath, "Route"),
		ResultIndex: resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func effectAxis() schema.EntryReference { return axisReference("effect") }

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func effectMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     effectFactorPackagePath,
		Name:            name,
		Receiver:        goType(effectFactorPackagePath, receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

// mountedCallProvider is the foreign candidate directory every row this rule
// declares hangs off: Effect owns which mounted calls exist, and no relation
// here mirrors that directory locally.
func mountedCallProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/mounted-call/candidates",
	})
}

// publicationFreezeRoutes is the exact Heap routes one publication call
// justifies freezing: the Recent allocation roots every known operation
// alternative of the call agrees on.
//
// Its inputs are the candidate, the call fact, and the ordered cells the
// subject selection answered, because a route set computed from every subject
// cannot be built one subject at a time. Semantic uncertainty is an empty
// valid relation rather than a refusal: the rule then settles its
// authenticated empty selection instead of fabricating Heap state.
func publicationFreezeRoutes() definition.Relation {
	return definition.Relation{
		Name: "PublicationFreezeRoutes", Key: "heap/publication-freeze/routes", Axis: "heap",
		Subject: "PublicationFreezeRouteCarrier",
		Inputs: []definition.RelationInput{
			{Carrier: "CallCoordinateCarrier"},
			{Carrier: "CallFactCarrier"},
			{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
		},
		CandidateProvider: mountedCallProvider(),
		Derivation: definition.RelationDerivation{
			State: goType(recentPlanPackagePath, "Plan"),
			Build: freezeFunction("DerivePublicationFreezeRoutes"),
			Count: freezeFunction("PublicationFreezeRouteCount"),
			At:    freezeFunction("PublicationFreezeRouteAt"),
			StaticAxes: []schema.EntryReference{
				heapAxis(),
				axisReference("value"),
				axisReference("call"),
				effectAxis(),
				axisReference("pack"),
			},
		},
	}
}

// Contribution is the Heap publication-freeze rule's own member declaration.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-publication-freeze",
		Carriers: []definition.Carrier{
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: goType(callPackagePath, "CallCoordinate")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: goType(valuePackagePath, "Coordinate")},
			{Name: "PublicationFreezeRouteCarrier", Key: "carrier/heap/publication-freeze-route", Type: goType(recentPlanPackagePath, "Route")},
			{Name: "PublicationFreezeRouteTagCarrier", Key: "carrier/heap/publication-freeze-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
			publicationFreezeRoutes(),
		},
		Projections: []definition.Projection{
			{
				Name:              "PublicationFreezeRouteKey",
				Axis:              "heap",
				Key:               "heap/publication-freeze/route-key",
				Relation:          "PublicationFreezeRoutes",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Key,
				Result:            "HeapKeyCarrier",
				Accessor:          routeMethod("Coordinates", 0),
			},
			{
				// The tag a routed selection pairs its cells by. The routed
				// form hands the member reducer this tag rather than the
				// coordinate, so the fold admits it back through the schema
				// that issued it.
				Name:              "PublicationFreezeRouteTag",
				Axis:              "heap",
				Key:               "heap/publication-freeze/route-tag",
				Relation:          "PublicationFreezeRoutes",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Predicate,
				Result:            "PublicationFreezeRouteTagCarrier",
				Accessor:          routeMethod("Predicate", -1),
			},
			{
				// A freeze publishes back into the very root it read, so the
				// destination is the same coordinate under its own role.
				Name:              "PublicationFreezeRouteDestination",
				Axis:              "heap",
				Key:               "heap/publication-freeze/route-destination",
				Relation:          "PublicationFreezeRoutes",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          routeMethod("Coordinates", 1),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "PublicationFreezeReducer",
			Key:  "heap/reducer/publication-freeze",
			Inputs: []definition.ReducerInput{{
				Axis:         heapAxis(),
				Carrier:      "HeapFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Tag:          "PublicationFreezeRouteTagCarrier",
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "PublicationFreezeFact", ResultIndex: 0},
		}},
	}
}
