// Package memberdefinition is the generator-only owner source for the
// Placement Store rule's own reducer. It is imported by the member definition
// roster and by nothing at runtime, so the store package keeps its judgment
// and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const storePackagePath = "github.com/wippyai/go-lua/domain/placement/store"

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// Contribution is the Placement Store rule's reducer definition: the sealed
// route candidate, the exact Value source read, and the selected Placement
// cell the transition is applied to. The fold is irreducible - it
// authenticates the selected cell and moves it by the candidate's declared
// lifetime - so it is the one place Store's judgment lives.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-storage",
		Selections: []definition.Selection{{
			// The rows of StorageRoutes do not exist until the reads before this
			// one have delivered their cells, so an operation publishes them
			// and stamps each with StorageRouteTag. Its body is the owner judgment
			// named here, never a second copy of it.
			Name: "StorageRouteSelection", Key: "placement/store/route-selection",
			Relation: "StorageRoutes", Tag: "StorageRouteTag",
			Implementation: definition.GoSymbol{PackagePath: storePackagePath, Name: "ResolveRoute", ResultIndex: 0},
		}},
		Reducers: []definition.Reducer{{
			Name:      "StorageReducer",
			Key:       "placement/store/reducer/storage",
			Candidate: "StorageTransferCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         axisReference("value"),
					Carrier:      "ValueFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:         axisReference("placement"),
					Carrier:      "PlacementFactCarrier",
					Form:         member.ReadFormSelected,
					Multiplicity: member.MultiplicityOne,
					Tag:          "RouteTagCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{
				Axis:    axisReference("placement"),
				Carrier: "PlacementFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: storePackagePath, Name: "StorageFold", ResultIndex: 0},
		}},
	}
}
