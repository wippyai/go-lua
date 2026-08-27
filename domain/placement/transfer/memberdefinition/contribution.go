// Package memberdefinition is the generator-only owner source for the
// Placement transfer rule's own route relation, its projections, and its
// reducer. It is imported by the member definition roster and by nothing at
// runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	transferPackagePath  = "github.com/wippyai/go-lua/domain/placement/transfer"
	placementPackagePath = "github.com/wippyai/go-lua/domain/placement"
	callPackagePath      = "github.com/wippyai/go-lua/domain/call"
	heapPackagePath      = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func placementAxis() schema.EntryReference { return axisReference("placement") }

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func transferFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: transferPackagePath, Name: name, ResultIndex: 0}
}

func routeAccessor(name string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: transferPackagePath,
		Name:        name,
		Receiver:    goType(transferPackagePath, "Route"),
		ResultIndex: resultIndex,
	}
}

// mountedCallProvider is the foreign candidate directory every row this rule
// declares hangs off: Call owns which mounted calls exist, and Placement
// mirrors that directory nowhere.
func mountedCallProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}
}

// Contribution is the Placement transfer rule's own member declaration.
//
// The rule reads three things and folds one. Its candidate is a mounted call.
// Join zero is that call's own fact. Join one is the call's ordered mounted
// actuals, a nested member set Value publishes. Join two is the transfer
// route set derived from those two, and it is the only row here whose
// construction is still authored.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-transfer",
		Carriers: []definition.Carrier{
			// Placement's own two carriers, repeated verbatim from the axis
			// declaration so this rule's fold and projection shapes are
			// derivable from the contribution alone; composition refuses a
			// repeat that disagrees.
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key"), Capability: carrier.Equatable},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact"), Capability: carrier.Ascending},
			// The two route carriers this rule authors: the selected route row
			// and its owner-issued tag. Call's carriers are imported below;
			// composition keeps those authorities owned by Call.
			{Name: "TransferRouteCarrier", Key: "carrier/placement/transfer-route", Type: goType(transferPackagePath, "Route"), Capability: carrier.DecodeOnly},
			{Name: "TransferRouteTagCarrier", Key: "carrier/placement/transfer-route-tag", Type: definition.GoType{Name: "uint64"}, Capability: carrier.DecodeOnly},
		},
		CarrierRefs: []definition.CarrierReference{
			// The mounted Call coordinate and fact remain Call authorities;
			// these references are source aliases, not duplicate Placement
			// carrier declarations.
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"}, Carrier: "carrier/call/mounted-call"}, Type: goType(callPackagePath, "CallCoordinate")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"}, Carrier: "carrier/call/fact"}, Type: goType(callPackagePath, "Value")},
		},
		Relations: []definition.Relation{
			{
				// The transfer route set: the exact allocation roots every
				// authenticated MayDeliver payload of this call demands be
				// displaced to Send. Its inputs are the candidate, the call
				// fact, and the whole vector of the call's actuals - a demand
				// set computed from every actual cannot be handed one actual
				// at a time.
				Name:    "TransferRoutes",
				Key:     "placement/transfer/routes",
				Subject: "TransferRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "CallCoordinateCarrier"},
					{Carrier: "CallFactCarrier"},
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
				},
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Derivation: definition.RelationDerivation{
					State: goType(transferPackagePath, "RoutePlan"),
					Build: transferFunction("DeriveTransferRoutes"),
					Count: transferFunction("TransferRouteCount"),
					At:    transferFunction("TransferRouteAt"),
					StaticAxes: []schema.EntryReference{
						placementAxis(),
						axisReference("value"),
						axisReference("call"),
						axisReference("pack"),
					},
				},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "TransferRouteKey",
				Key:               "placement/transfer/route-key",
				Relation:          "TransferRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Key,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor("Coordinates", 0),
			},
			{
				// The route coordinate a member is published at, carrying the
				// Send policy the demand was authenticated under. The routed
				// worker pairs a cell with its member by this tag, and the
				// fold reads the policy out of the same tag rather than
				// deriving a second one.
				Name:              "TransferRouteTag",
				Key:               "placement/transfer/route-tag",
				Relation:          "TransferRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Predicate,
				Result:            "TransferRouteTagCarrier",
				Accessor:          routeAccessor("Predicate", -1),
			},
			{
				// A transfer displaces the world at the very root it read, so
				// the destination is the same coordinate under its own role.
				Name:              "TransferRouteDestination",
				Key:               "placement/transfer/route-destination",
				Relation:          "TransferRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Destination,
				Result:            "PlacementKeyCarrier",
				Accessor:          routeAccessor("Coordinates", 1),
			},
		},
		Selections: []definition.Selection{{
			// The rows of TransferRoutes do not exist until the reads before this
			// one have delivered their cells, so an operation publishes them
			// and stamps each with TransferRouteTag. Its body is the owner judgment
			// named here, never a second copy of it.
			Name: "TransferRouteSelection", Key: "placement/transfer/route-selection",
			Relation: "TransferRoutes", Tag: "TransferRouteTag",
			Implementation: transferFunction("DeriveTransferRoutes"),
		}},
		Reducers: []definition.Reducer{{
			Name: "TransferReducer",
			Key:  "placement/transfer/reducer",
			Inputs: []definition.ReducerInput{{
				Axis:         placementAxis(),
				Carrier:      "PlacementFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Tag:          "TransferRouteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis(), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: transferPackagePath, Name: "TransferFold", ResultIndex: 0},
		}},
	}
}
