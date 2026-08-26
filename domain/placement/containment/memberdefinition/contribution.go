// Package memberdefinition is the generator-only source for containment's
// owner-derived route relation and irreducible Placement reducer. The route
// relation owns the complete vector deliveries; the reducer receives only its
// selected child and retained-parent scalar columns.
package memberdefinition

import (
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
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

func containmentFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: containmentPackagePath, Name: name, ResultIndex: 0}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: containmentPackagePath, Name: name, Receiver: goType("Route"), ResultIndex: result}
}

// provider is the candidate authority this rule's rows hang off: the mounted
// point Program issues the rule at, reached through its entry-geometry row.
func provider() member.CandidateRef {
	return member.IssuedRowCandidate(programissuance.RelationOccurrenceEntryGeometry)
}

// Contribution names the scalar fold and the owner-derived route row that
// supplies it. The complete Placement/Heap vectors are the route derivation's
// inputs; each immutable route retains its authenticated parent Fact, so the
// fold receives two scalars from one selected row and never scans or rebuilds
// either vector.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-containment",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: definition.GoType{PackagePath: heapPackagePath, Name: "Key"}, Capability: carrier.Equatable},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Fact"}, Capability: carrier.Ascending},
			{Name: "ContainmentRouteCarrier", Key: "carrier/placement/containment-route", Type: goType("Route"), Capability: carrier.DecodeOnly},
			{Name: "ContainmentRouteTagCarrier", Key: "carrier/placement/containment-route-tag", Type: definition.GoType{Name: "uint64"}, Capability: carrier.DecodeOnly},
		},
		CarrierRefs: []definition.CarrierReference{
			{Name: "HeapFactCarrier", Key: "carrier/heap/fact", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}, Carrier: "carrier/heap/fact"}, Type: definition.GoType{PackagePath: heapPackagePath, Name: "Value"}},
			{Name: "HeapKeyCarrier", Key: "carrier/heap/key", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}, Carrier: "carrier/heap/key"}, Type: definition.GoType{PackagePath: heapPackagePath, Name: "Key"}},
		},
		Relations: []definition.Relation{
			{
				// Placement's complete summary is an owner-issued vector. The
				// route carrier supplies its authenticated coordinate column.
				Name: "ContainmentPlacementSummary", Key: "placement/containment/placement-summary",
				Subject: "ContainmentRouteCarrier", Inputs: []definition.RelationInput{{Carrier: "PlacementFactCarrier", Many: true, Form: member.ReadFormComplete}},
				Addressing:        member.Addressing{Address: "placement/containment/placement-summary-coordinate"},
				CandidateProvider: provider(),
			},
			{
				// The paired Heap summary is still declared at the containment
				// owner. Roster composition moves this row to Heap while keeping
				// HeapKeyCarrier an explicit imported authority.
				Name: "ContainmentHeapSummary", Key: "heap/containment/heap-summary", Axis: "heap",
				Subject: "ContainmentRouteCarrier", Inputs: []definition.RelationInput{{Carrier: "HeapFactCarrier", Many: true, Form: member.ReadFormComplete}},
				Addressing:        member.Addressing{Address: "heap/containment/heap-summary-coordinate"},
				CandidateProvider: provider(),
			},
			{
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
				Name: "ContainmentPlacementKey", Key: "placement/containment/placement-summary-coordinate",
				Relation: "ContainmentPlacementSummary", Role: member.Key, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
			},
			{
				Name: "ContainmentHeapKey", Key: "heap/containment/heap-summary-coordinate", Axis: "heap",
				Relation: "ContainmentHeapSummary", Role: member.Key, Result: "HeapKeyCarrier",
				Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
			},
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
			{
				Name: "ContainmentRouteParent", Key: "placement/containment/route-parent",
				Relation: "ContainmentRoutes", Role: member.Attribute, Result: "PlacementFactCarrier",
				Accessor: routeMethod("Parent", -1), CandidateProvider: provider(),
			},
		},
		Selections: []definition.Selection{{
			// The edge set is the walk of every authenticated parent's Heap
			// containments, widened to every root where that value is opaque,
			// so it does not exist until both vectors are known.
			Name: "ContainmentRouteSelection", Key: "placement/containment/route-selection",
			Relation: "ContainmentRoutes", Tag: "ContainmentRouteTag",
			Implementation: containmentFunction("DeriveContainmentRoutes"),
		}},
		Reducers: []definition.Reducer{{
			Name: "ContainmentReducer",
			Key:  "placement/containment/reducer",
			Inputs: []definition.ReducerInput{
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne},
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne},
			},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: containmentPackagePath, Name: "ContainmentFold", ResultIndex: 0},
		}},
	}
}
