// Package memberdefinition is the generator-only contribution for Placement's
// closure-capture rule: the rows its Program names, the operations that
// publish the produced ones, and its fold signature. It names no runtime rule
// protocol and creates no second route authority.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

const (
	capturePackagePath = "github.com/wippyai/go-lua/domain/placement/capture"
	heapPackagePath    = "github.com/wippyai/go-lua/domain/heap"
	valuePackagePath   = "github.com/wippyai/go-lua/domain/value"
)

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func captureFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: capturePackagePath, Name: name, ResultIndex: 0}
}

func sourceMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: capturePackagePath, Name: name, Receiver: goType(capturePackagePath, "Source"), ResultIndex: result}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: capturePackagePath, Name: name, Receiver: goType(capturePackagePath, "Route"), ResultIndex: result}
}

// provider is the candidate authority both dependent relations hang off:
// Program's authenticated closure-proof row for this allocation occurrence.
// Neither relation mints a directory of its own.
func provider() member.CandidateRef {
	return member.IssuedRowCandidate(programissuance.RelationOccurrenceClosureProof)
}

// Contribution declares the closure-capture judgment together with the rows it
// reads. The capture-source vector is Value data whichever rule names it, so
// it is declared on the Value axis and the roster places it there; the route
// vector and the fold are Placement's own.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-closure-capture",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType("github.com/wippyai/go-lua/domain/placement", "Fact")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: goType(valuePackagePath, "Coordinate")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
			{Name: "CaptureSourceCarrier", Key: "carrier/value/closure-capture-source", Type: goType(capturePackagePath, "Source")},
			{Name: "CaptureSourceTagCarrier", Key: "carrier/value/closure-capture-source-tag", Type: definition.GoType{Name: "uint64"}},
			{Name: "CaptureRouteCarrier", Key: "carrier/placement/capture-route", Type: goType(capturePackagePath, "Route")},
			{Name: "CaptureRouteTagCarrier", Key: "carrier/placement/capture-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
			{
				// One row per declared capture of the closure, carrying the
				// Value coordinate of the cell it closes over.
				Name: "CaptureSources", Key: "value/closure-capture/sources", Axis: "value",
				Subject:           "CaptureSourceCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "PlacementKeyCarrier"}},
				CandidateProvider: provider(),
			},
			{
				// One row per allocation the closure's captured values reach.
				// The set is computed from the Value cells the source read
				// delivered, which is why an operation publishes it.
				Name: "CaptureRoutes", Key: "placement/closure-capture/routes",
				Subject: "CaptureRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "PlacementKeyCarrier"},
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSelected},
				},
				CandidateProvider: provider(),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "CaptureSourceKey", Key: "value/closure-capture/source-key", Axis: "value",
				Relation: "CaptureSources", Role: member.Key, Result: "ValueCoordinateCarrier",
				Accessor: sourceMethod("Coordinate", -1), CandidateProvider: provider(),
			},
			{
				Name: "CaptureSourceTag", Key: "value/closure-capture/source-tag", Axis: "value",
				Relation: "CaptureSources", Role: member.Predicate, Result: "CaptureSourceTagCarrier",
				Accessor: sourceMethod("Predicate", -1), CandidateProvider: provider(),
			},
			{
				Name: "CaptureRouteKey", Key: "placement/closure-capture/route-key",
				Relation: "CaptureRoutes", Role: member.Key, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
			},
			{
				Name: "CaptureRouteTag", Key: "placement/closure-capture/route-tag",
				Relation: "CaptureRoutes", Role: member.Predicate, Result: "CaptureRouteTagCarrier",
				Accessor: routeMethod("Predicate", -1), CandidateProvider: provider(),
			},
			{
				Name: "CaptureRouteDestination", Key: "placement/closure-capture/route-destination",
				Relation: "CaptureRoutes", Role: member.Destination, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 1), CandidateProvider: provider(),
			},
		},
		Selections: []definition.Selection{
			{
				// The capture rows exist only once the closure's boundary has
				// been resolved through the artifact that mounted it, so they
				// are published by this operation rather than enumerated.
				Name: "CaptureSourceSelection", Key: "value/closure-capture/source-selection",
				Relation: "CaptureSources", Tag: "CaptureSourceTag",
				Implementation: captureFunction("DeriveCaptureSources"),
			},
			{
				// The route set is the union of the allocations every captured
				// Value cell reaches, so it does not exist until those cells
				// are known.
				Name: "CaptureRouteSelection", Key: "placement/closure-capture/route-selection",
				Relation: "CaptureRoutes", Tag: "CaptureRouteTag",
				Implementation: captureFunction("DeriveCaptureRoutes"),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "CaptureReducer",
			Key:  "placement/closure-capture/reducer",
			Inputs: []definition.ReducerInput{
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne},
				{Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "CaptureRouteTagCarrier"},
			},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: capturePackagePath, Name: "CaptureFold", ResultIndex: 0},
		}},
	}
}
