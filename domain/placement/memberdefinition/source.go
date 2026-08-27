// Package memberdefinition is the generator-only owner source for
// Placement's Store route vocabulary.  The route candidate directory remains
// Value-owned; this source declares only the Placement-side dependent row,
// projections, and irreducible Store fold signature.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	placementPackagePath = "github.com/wippyai/go-lua/domain/placement"
	heapPackagePath      = "github.com/wippyai/go-lua/domain/heap"
	storePackagePath     = "github.com/wippyai/go-lua/domain/placement/store"
	valuePackagePath     = "github.com/wippyai/go-lua/domain/value"
	identityPackagePath  = "github.com/wippyai/go-lua/analysis/identity"
)

func placementGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: placementPackagePath, Name: name}
}

func heapGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: heapPackagePath, Name: name}
}

func storeGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: storePackagePath, Name: name}
}

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func identityGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: identityPackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func placementMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     placementPackagePath,
		Name:            name,
		Receiver:        placementGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func storeMethod(name string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: storePackagePath,
		Name:        name,
		Receiver:    storeGoType("Route"),
		ResultIndex: resultIndex,
	}
}

func storeFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: storePackagePath, Name: name, ResultIndex: 0}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func candidateProvider() member.RelationRef {
	return member.RelationRef{
		Axis:   axisReference("value"),
		Member: "value/storage-transfer/candidates",
	}
}

// Storage returns Placement's authored Store route vocabulary.  Value owns
// StorageTransferCandidates; Placement owns only the dependent route row and
// its projections.  The explicit foreign provider is load-bearing: no Go-type
// inference or local candidate mirror may be introduced by composition.
func Storage() definition.Definition {
	provider := candidateProvider()
	return definition.Definition{
		Name:       "PlacementStorage",
		Axis:       "placement",
		ImportPath: "github.com/wippyai/go-lua/domain/placement",
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "PlacementKeyCarrier",
			Dense:      builtinGoType("uint32"),
			Normalizer: placementMethod("KeyIndex", "Schema", false, 0),
		}},
		Signature: definition.Signature{
			Key:  "PlacementKeyCarrier",
			Fact: "PlacementFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: heapGoType("Key"), Capability: carrier.Equatable},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: placementGoType("Fact"), Capability: carrier.Ascending},
			{Name: "AllocationEvidenceCarrier", Key: "carrier/placement/allocation-evidence", Type: placementGoType("AllocationEvidence"), Capability: carrier.DecodeOnly},
			{Name: "PlacementSchemaIDCarrier", Key: "carrier/placement/schema-id", Type: identityGoType("ContentID"), Capability: carrier.DecodeOnly},
			{Name: "StorageRouteCarrier", Key: "carrier/placement/storage-route", Type: storeGoType("Route"), Capability: carrier.DecodeOnly},
			{Name: "RouteTagCarrier", Key: "carrier/placement/storage-route-tag", Type: builtinGoType("uint64"), Capability: carrier.DecodeOnly},
		},
		CarrierRefs: []definition.CarrierReference{
			{Name: "StorageTransferCarrier", Key: "carrier/value/storage-transfer", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Carrier: "carrier/value/storage-transfer"}, Type: valueGoType("StorageTransfer")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Carrier: "carrier/value/fact"}, Type: valueGoType("Value")},
		},
		Enumerations: []definition.Enumeration{
			{
				// Over nothing: the owner's whole coordinate directory, which
				// is what a route set widens to when the value it is derived
				// from names no closed list of allocations. It yields in this
				// axis's own dense order, so a set ordered by this axis IS the
				// directory and reading it copies nothing.
				Name: "AllocationDirectory", Item: "PlacementKeyCarrier",
				Count: placementMethod("DenseKeyCount", "Schema", false, -1),
				At:    placementMethod("KeyAt", "Schema", false, 0),
				Order: axisReference("placement"),
			},
		},
		Relations: []definition.Relation{
			{
				Name:              "StorageRoutes",
				Key:               "placement/store/storage-routes",
				Subject:           "StorageRouteCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "StorageTransferCarrier"}, {Carrier: "ValueFactCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(provider),
				// Declared rather than authored: what to enumerate, how to
				// union it, what to widen to when the value names no closed
				// list of allocations, and the order the routes come back in
				// are all stated here and written by the emitter. What is left
				// authored is what only this domain can answer - what one atom
				// of a Value means to Placement, what one row of Placement's
				// own directory means, and when there is no list to enumerate
				// at all.
				Derivation: definition.RelationDerivation{
					StaticAxes: []schema.EntryReference{
						axisReference("placement"),
						axisReference("value"),
					},
					Source:  []definition.EnumerationRef{{Axis: axisReference("value"), Name: "Atoms"}},
					Resolve: storeFunction("ResolveRoute"),
					// A store transfer routes to one allocation, or to a
					// couple where a value carries alternatives; a wider
					// answer is the widened one, which is not held by value at
					// all.
					InlineWidth: 8,
					Widen: definition.DerivationWiden{
						Predicate: storeFunction("BeyondAllocations"),
						Source:    []definition.EnumerationRef{{Axis: axisReference("placement"), Name: "AllocationDirectory"}},
						// A directory row is a Heap key, not an atom of a
						// Value, so the endpoint answers what one of those
						// means with its own judgment.
						Resolve: storeFunction("ResolveDirectoryRoute"),
					},
				},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "StorageRouteKey",
				Key:               "placement/store/route-key",
				Relation:          "StorageRoutes",
				Role:              member.Key,
				Result:            "PlacementKeyCarrier",
				Accessor:          storeMethod("Coordinates", 0),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name:              "StorageRouteTag",
				Key:               "placement/store/route-tag",
				Relation:          "StorageRoutes",
				Role:              member.Predicate,
				Result:            "RouteTagCarrier",
				Accessor:          storeMethod("Predicate", -1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name:              "StorageRouteDestination",
				Key:               "placement/store/route-destination",
				Relation:          "StorageRoutes",
				Role:              member.Destination,
				Result:            "PlacementKeyCarrier",
				Accessor:          storeMethod("Coordinates", 1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
		},
	}
}
