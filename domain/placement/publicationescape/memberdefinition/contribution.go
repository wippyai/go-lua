// Package memberdefinition is the generator-only source for Publication
// Escape: the two vectors its Program reads, the operations that publish
// them, and the irreducible Placement judgment that folds the routed cell.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
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

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func sourceMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: name, Receiver: goType(publicationEscapePackagePath, "Source"), ResultIndex: result}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: name, Receiver: goType(publicationEscapePackagePath, "Route"), ResultIndex: result}
}

// provider is the candidate authority both vectors hang off: Effect's own
// mounted call row. The publication batch is data reached from that row, so
// neither vector mints a second candidate directory.
func provider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis:   axisReference("effect"),
		Member: effectdomain.MountedEffectCallCandidates,
	})
}

// Contribution declares Publication Escape's own rows.
//
// The source vector is Effect data whichever rule reads it, so it is declared
// on the Effect axis and the roster places it there; the route vector and the
// fold are Placement's. Unknown is not a default anywhere here: it is a value
// the route rows carry for an open or opaque subject only.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-publication-escape",
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact")},
			{Name: "PublicationRequirementCarrier", Key: "carrier/placement/publication-requirement", Type: goType(placementPackagePath, "Placement")},
			{Name: "EffectMountedCallCarrier", Key: "carrier/effect/mounted-call", Type: goType(effectFactorPackagePath, "MountedCall")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: goType(valuePackagePath, "Coordinate")},
			{Name: "PublicationSourceCarrier", Key: "carrier/effect/mounted-publication-source", Type: goType(publicationEscapePackagePath, "Source")},
			{Name: "PublicationSourceTagCarrier", Key: "carrier/effect/mounted-publication-source-tag", Type: definition.GoType{Name: "uint64"}},
			{Name: "PublicationRouteCarrier", Key: "carrier/placement/publication-escape-route", Type: goType(publicationEscapePackagePath, "Route")},
			{Name: "PublicationRouteTagCarrier", Key: "carrier/placement/publication-escape-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
			{
				// One row per subject member of every publication the call's
				// own fact authorized. Which publications those are is not
				// known until that fact is read, so the rows are produced.
				Name: "PublicationSources", Key: "effect/mounted-publication/sources", Axis: "effect",
				Subject: "PublicationSourceCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "EffectMountedCallCarrier"},
					{Carrier: "CallFactCarrier"},
				},
				CandidateProvider: provider(),
			},
			{
				// One row per allocation root the authorized publications
				// reach, widened to every root where the subject is open. The
				// set is computed from the Value cells the source read
				// delivered.
				Name: "PublicationRoutes", Key: "placement/publication-escape/routes",
				Subject: "PublicationRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "EffectMountedCallCarrier"},
					{Carrier: "CallFactCarrier"},
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSelected},
				},
				CandidateProvider: provider(),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "PublicationSourceKey", Key: "effect/mounted-publication/source-value-coordinate", Axis: "effect",
				Relation: "PublicationSources", Role: member.Key, Result: "ValueCoordinateCarrier",
				Accessor: sourceMethod("Coordinate", 0), CandidateProvider: provider(),
			},
			{
				Name: "PublicationSourceTag", Key: "effect/mounted-publication/source-tag", Axis: "effect",
				Relation: "PublicationSources", Role: member.Predicate, Result: "PublicationSourceTagCarrier",
				Accessor: sourceMethod("Predicate", 0), CandidateProvider: provider(),
			},
			{
				Name: "PublicationRouteKey", Key: "placement/publication-escape/route-key",
				Relation: "PublicationRoutes", Role: member.Key, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
			},
			{
				// The route rows carry their own tag. The reading rule states
				// no predicate over them - a RouteMember pairs the tag with the
				// destination, and a second one would be a duplicate route
				// plan - but the operation that publishes the rows stamps this
				// column, so the axis declares it.
				Name: "PublicationRouteTag", Key: "placement/publication-escape/route-tag",
				Relation: "PublicationRoutes", Role: member.Predicate, Result: "PublicationRouteTagCarrier",
				Accessor: routeMethod("Predicate", -1), CandidateProvider: provider(),
			},
			{
				Name: "PublicationRouteDestination", Key: "placement/publication-escape/route-destination",
				Relation: "PublicationRoutes", Role: member.Destination, Result: "PlacementKeyCarrier",
				Accessor: routeMethod("Coordinates", 1), CandidateProvider: provider(),
			},
		},
		Selections: []definition.Selection{
			{
				Name: "PublicationSourceSelection", Key: "effect/publication-escape/source-selection",
				Relation: "PublicationSources", Tag: "PublicationSourceTag",
			},
			{
				Name: "PublicationRouteSelection", Key: "placement/publication-escape/route-selection",
				Relation: "PublicationRoutes", Tag: "PublicationRouteTag",
			},
		},
		Reducers: []definition.Reducer{{
			Name: "PublicationEscapeReducer",
			Key:  "placement/publication-escape/reducer",
			Inputs: []definition.ReducerInput{{
				Axis: placementAxis(), Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne, Tag: "PublicationRequirementCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: publicationEscapePackagePath, Name: "PublicationEscapeFold", ResultIndex: 0},
		}},
	}
}
