// Package memberdefinition is the generator-only source for Publication
// Escape's irreducible Placement reducer.  Effect batch and Value subject
// relation rows remain foreign authorities; this contribution adds only the
// Placement policy carrier required by the selected route predicate.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const publicationEscapePackagePath = "github.com/wippyai/go-lua/domain/placement/publicationescape"

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

// Contribution declares the direct call shape
// PublicationEscapeFold(requirement, selectedFact) -> (fact, outcome).
// Unknown is not a default: it is a value supplied by the authenticated route
// relation for an open/opaque subject only.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-publication-escape",
		Carriers: []definition.Carrier{{
			Name: "PublicationRequirementCarrier",
			Key:  "carrier/placement/publication-requirement",
			Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Placement"},
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
