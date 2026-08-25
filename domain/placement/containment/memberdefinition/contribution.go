// Package memberdefinition is the generator-only source for containment's
// irreducible Placement reducer.  Dependent route rows remain an FT-25 seam;
// this contribution therefore declares only the scalar judgment that the
// current member ABI can type without a vector facade.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const containmentPackagePath = "github.com/wippyai/go-lua/domain/placement/containment"

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

// Contribution names the scalar fold.  The complete Placement/Heap vectors
// are the inputs to the not-yet-landed dependent Build; once the vector-view
// ABI is wired, that Build supplies the authenticated parent cell to this
// reducer rather than making the reducer scan or reconstruct either vector.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-containment",
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
