// Package memberdefinition is the generator-only source for containment's
// irreducible Placement reducer.  Dependent route rows remain an FT-25 seam;
// this contribution therefore declares only the scalar judgment that the
// current member ABI can type without a vector facade.
package memberdefinition

import (
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	containmentPackagePath = "github.com/wippyai/go-lua/domain/placement/containment"
	heapPackagePath        = "github.com/wippyai/go-lua/domain/heap"
)

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

func goType(name string) definition.GoType {
	return definition.GoType{PackagePath: containmentPackagePath, Name: name}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: containmentPackagePath, Name: name, Receiver: goType("Route"), ResultIndex: result}
}

// provider is the candidate authority this rule's rows hang off: the mounted
// point Program issues the rule at, reached through its entry-geometry row.
func provider() member.CandidateRef {
	return member.IssuedRowCandidate(programissuance.RelationOccurrenceEntryGeometry)
}

// Contribution names the scalar fold.  The complete Placement/Heap vectors
// are the inputs to the not-yet-landed dependent Build; once the vector-view
// ABI is wired, that Build supplies the authenticated parent cell to this
// reducer rather than making the reducer scan or reconstruct either vector.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-containment",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: definition.GoType{PackagePath: heapPackagePath, Name: "Key"}},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Fact"}},
			{Name: "HeapFactCarrier", Key: "carrier/heap/fact", Type: definition.GoType{PackagePath: heapPackagePath, Name: "Value"}},
			{Name: "ContainmentRouteCarrier", Key: "carrier/placement/containment-route", Type: goType("Route")},
			{Name: "ContainmentRouteTagCarrier", Key: "carrier/placement/containment-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{{
			// One row per parent-to-child containment edge. Which edges exist
			// depends on the two complete vectors read before this one, so the
			// rows are produced rather than enumerated.
			Name: "ContainmentRoutes", Key: "placement/containment/routes",
			Subject: "ContainmentRouteCarrier",
			Inputs: []definition.RelationInput{
				{Carrier: "PlacementFactCarrier", Many: true, Form: member.ReadFormComplete},
				{Carrier: "HeapFactCarrier", Many: true, Form: member.ReadFormComplete},
			},
			CandidateProvider: provider(),
		}},
		Projections: []definition.Projection{
			{
				Name: "ContainmentRouteKey", Key: "placement/containment/route-key",
				Relation: "ContainmentRoutes", Role: member.Key, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
			},
			{
				Name: "ContainmentRouteTag", Key: "placement/containment/route-tag",
				Relation: "ContainmentRoutes", Role: member.Predicate, Result: "ContainmentRouteTagCarrier",
				Accessor: routeMethod("Predicate", -1), CandidateProvider: provider(),
			},
			{
				Name: "ContainmentRouteDestination", Key: "placement/containment/route-destination",
				Relation: "ContainmentRoutes", Role: member.Destination, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 1), CandidateProvider: provider(),
			},
		},
		Selections: []definition.Selection{{
			// The edge set is the walk of every authenticated parent's Heap
			// containments, widened to every root where that value is opaque,
			// so it does not exist until both vectors are known.
			Name: "ContainmentRouteSelection", Key: "placement/containment/route-selection",
			Relation: "ContainmentRoutes", Tag: "ContainmentRouteTag",
		}},
		Reducers: []definition.Reducer{{
			Name: "ContainmentReducer",
			Key:  "placement/containment/reducer",
			Inputs: []definition.ReducerInput{
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne},
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
			},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: containmentPackagePath, Name: "ContainmentFold", ResultIndex: 0},
		}},
	}
}
