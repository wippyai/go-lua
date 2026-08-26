// Package memberdefinition is the generator-only owner source for Placement
// Publication Escape's route relation and reducer.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	effectdomain "github.com/wippyai/go-lua/domain/effect"
)

const (
	publicationEscapePackagePath = "github.com/wippyai/go-lua/domain/placement/publicationescape"
	placementPackagePath         = "github.com/wippyai/go-lua/domain/placement"
	effectFactorPackagePath      = "github.com/wippyai/go-lua/domain/effect/factor"
	callPackagePath              = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath             = "github.com/wippyai/go-lua/domain/value"
	heapPackagePath              = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func placementAxis() schema.EntryReference { return axisReference("placement") }

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func provider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("effect"), Member: effectdomain.MountedEffectCallCandidates}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: name, Receiver: goType(publicationEscapePackagePath, "Route"), ResultIndex: result}
}

func routeFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: name, ResultIndex: 0}
}

// Contribution declares the complete owner-issued route vertical. Effect
// owns the candidate and publication source relation; this Placement owner
// derives routes from the pre-seal composition values and publishes Key, Tag,
// and Destination from the same Route row.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement", Rule: "placement-publication-escape",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key"), Capability: carrier.Equatable},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact"), Capability: carrier.Ascending},
			{Name: "PublicationRequirementCarrier", Key: "carrier/placement/publication-requirement", Type: goType(placementPackagePath, "Placement"), Capability: carrier.DecodeOnly},
			{Name: "PublicationRouteCarrier", Key: "carrier/placement/publication-escape-route", Type: goType(publicationEscapePackagePath, "Route"), Capability: carrier.DecodeOnly},
			{Name: "PublicationRouteTagCarrier", Key: "carrier/placement/publication-escape-route-tag", Type: definition.GoType{Name: "uint64"}, Capability: carrier.DecodeOnly},
		},
		CarrierRefs: []definition.CarrierReference{
			{Name: "EffectMountedCallCarrier", Key: "carrier/effect/mounted-call", Ref: carrier.Ref{Owner: axisReference("effect"), Carrier: "carrier/effect/mounted-call"}, Type: goType(effectFactorPackagePath, "MountedCall")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Ref: carrier.Ref{Owner: axisReference("call"), Carrier: "carrier/call/fact"}, Type: goType(callPackagePath, "Value")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Ref: carrier.Ref{Owner: axisReference("value"), Carrier: "carrier/value/fact"}, Type: goType(valuePackagePath, "Value")},
		},
		Relations: []definition.Relation{{
			Name: "PublicationRoutes", Key: "placement/publication-escape/routes", Subject: "PublicationRouteCarrier",
			Inputs: []definition.RelationInput{
				{Carrier: "EffectMountedCallCarrier"},
				{Carrier: "CallFactCarrier"},
				{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSelected},
			},
			CandidateProvider: member.AxisRelationCandidate(provider()),
			Derivation: definition.RelationDerivation{
				State:      goType(publicationEscapePackagePath, "RoutePlan"),
				Build:      routeFunction("DerivePublicationRoutesFromComposition"),
				Count:      routeFunction("PublicationRouteCount"),
				At:         routeFunction("PublicationRouteAt"),
				StaticAxes: []schema.EntryReference{placementAxis(), axisReference("value"), axisReference("effect"), axisReference("call")},
			},
		}},
		Projections: []definition.Projection{
			{Name: "PublicationRouteKey", Key: "placement/publication-escape/route-key", Relation: "PublicationRoutes", Role: member.Key, Result: "PlacementKeyCarrier", Accessor: routeMethod("Coordinates", 0), CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "PublicationRouteTag", Key: "placement/publication-escape/route-tag", Relation: "PublicationRoutes", Role: member.Predicate, Result: "PublicationRouteTagCarrier", Accessor: routeMethod("Predicate", 0), CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "PublicationRouteDestination", Key: "placement/publication-escape/route-destination", Relation: "PublicationRoutes", Role: member.Destination, Result: "PlacementKeyCarrier", Accessor: routeMethod("Coordinates", 1), CandidateProvider: member.AxisRelationCandidate(provider())},
		},
		Selections: []definition.Selection{{
			Name: "PublicationRouteSelection", Key: "placement/publication-escape/route-selection", Relation: "PublicationRoutes", Tag: "PublicationRouteTag", Implementation: routeFunction("DerivePublicationRoutesFromComposition"),
		}},
		Reducers: []definition.Reducer{{
			Name: "PublicationEscapeReducer", Key: "placement/publication-escape/reducer",
			Inputs:         []definition.ReducerInput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "PublicationRequirementCarrier"}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: "PublicationEscapeFold", ResultIndex: 0},
		}},
	}
}
