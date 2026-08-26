// Package memberdefinition is the generator-only source for Publication
// Escape's irreducible Placement reducer.  Effect batch and Value subject
// relation rows remain foreign authorities; this contribution adds only the
// Placement policy carrier required by the selected route predicate.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	publicationEscapePackagePath = "github.com/wippyai/go-lua/domain/placement/publicationescape"
)

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

func routeProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "effect"},
		Member: "effect/mounted-call/candidates",
	})
}

func routeType(name string) definition.GoType {
	return definition.GoType{PackagePath: publicationEscapePackagePath, Name: name}
}

func routeMethod(name string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: publicationEscapePackagePath,
		Name:        name,
		Receiver:    routeType("plannedRoute"),
		ResultIndex: resultIndex,
	}
}

// Contribution declares the direct call shape
// PublicationEscapeFold(requirement, selectedFact) -> (fact, outcome).
// Unknown is not a default: it is a value supplied by the authenticated route
// relation for an open/opaque subject only.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-publication-escape",
		Carriers: []definition.Carrier{
			{Name: "PublicationRequirementCarrier", Key: "carrier/placement/publication-requirement", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Placement"}, Capability: carrier.DecodeOnly},
			{Name: "PublicationRouteCarrier", Key: "carrier/placement/publication-escape-route", Type: routeType("plannedRoute"), Capability: carrier.DecodeOnly},
			{Name: "PublicationRouteTagCarrier", Key: "carrier/placement/publication-escape-route-tag", Type: definition.GoType{Name: "uint64"}, Capability: carrier.DecodeOnly},
		},
		Relations: []definition.Relation{{
			// Route rows are produced by the one pre-seal publication route
			// construction held by the hot rule. The relation declaration names
			// that owner-issued row and its foreign mounted-call candidate; it
			// does not add a second planner or candidate directory.
			Name:              "PublicationRoutes",
			Key:               "placement/publication-escape/routes",
			Subject:           "PublicationRouteCarrier",
			CandidateProvider: routeProvider(),
		}},
		Projections: []definition.Projection{{
			Name:              "PublicationRouteDestination",
			Key:               "placement/publication-escape/route-destination",
			Relation:          "PublicationRoutes",
			Role:              member.Destination,
			Result:            "PlacementKeyCarrier",
			Accessor:          routeMethod("Coordinates", 1),
			CandidateProvider: routeProvider(),
		}},
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
