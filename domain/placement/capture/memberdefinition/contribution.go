// Package memberdefinition is the generator-only contribution for Placement's
// closure-capture reducer. It names the fold signature without importing any
// runtime rule protocol or creating a second route authority.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const capturePackagePath = "github.com/wippyai/go-lua/domain/placement/capture"

func placementAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

// Contribution declares the containment judgment. The Value capture-source
// vector belongs to the route derivation; the reducer consumes only the
// Placement parent and selected routed predecessor.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-closure-capture",
		Carriers: []definition.Carrier{
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: definition.GoType{PackagePath: "github.com/wippyai/go-lua/domain/placement", Name: "Fact"}},
			{Name: "CaptureRouteTagCarrier", Key: "carrier/placement/capture-route-tag", Type: definition.GoType{Name: "uint64"}},
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
