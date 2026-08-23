// Package memberdefinition is the generator-only owner source for Heap's
// transformed-carry member vocabulary. The runtime heap package imports only
// the generated cold catalog; this package carries no executable handle or
// runtime callback.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	heapPackagePath   = "github.com/wippyai/go-lua/domain/heap"
	sourcePackagePath = "github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
)

func heapGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: heapPackagePath, Name: name}
}

func sourceGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: sourcePackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func heapMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     heapPackagePath,
		Name:            name,
		Receiver:        heapGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func sourceMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: sourcePackagePath,
		Name:        name,
		Receiver:    definition.GoType{PackagePath: sourcePackagePath, Name: receiver},
		ResultIndex: resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// AllocationCarry returns Heap's two distinct allocation-form carry members.
// Both candidate methods apply Heap.Schema.Age, but the member keys and
// candidate relationships remain separate semantic identities for empty and
// closed allocation rules.
func AllocationCarry() definition.Definition {
	return definition.Definition{
		Name: "HeapAllocationCarry",
		Axis: "heap",
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "HeapKeyCarrier",
			Dense:      builtinGoType("uint32"),
			Normalizer: heapMethod("DenseKeyIndex", "Schema", false, 0),
		}},
		Signature: definition.Signature{Key: "HeapKeyCarrier", Fact: "HeapFactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "HeapKeyCarrier", Key: "carrier/heap/key", Type: heapGoType("Key")},
			{Name: "HeapFactCarrier", Key: "carrier/heap/fact", Type: heapGoType("Value")},
			{Name: "EmptyAllocationCarrier", Key: "carrier/heap/allocation-empty", Type: sourceGoType("Root")},
			{Name: "ClosedAllocationCarrier", Key: "carrier/heap/allocation-closed", Type: sourceGoType("Closed")},
		},
		Relations: []definition.Relation{
			{
				Name:              "IngressSeeds",
				Key:               "heap/ingress/candidates",
				Subject:           "HeapKeyCarrier",
				CandidateProvider: member.RelationRef{Axis: axisReference("heap"), Member: "heap/ingress/candidates"},
				CandidateResolver: heapMethod("AllocationRootForMountedOccurrence", "Schema", false, 0),
				CandidateOrdinal:  heapMethod("AllocationRootOrdinal", "Schema", false, 0),
				CandidateAt:       heapMethod("AllocationRootAt", "Schema", false, 0),
				CandidateCount:    heapMethod("AllocationRootCount", "Schema", false, 0),
				Materialize:       definition.GoSymbol{PackagePath: heapPackagePath, Name: "IngressFact", ResultIndex: 0},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "IngressCoordinate",
				Key:               "heap/ingress/coordinate",
				Relation:          "IngressSeeds",
				CandidateProvider: member.RelationRef{Axis: axisReference("heap"), Member: "heap/ingress/candidates"},
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("Ingress", "Key", false, 0),
			},
		},
		CarryTransforms: []definition.CarryTransform{
			{
				Name:           "EmptyAllocationCarryTransform",
				Key:            "transform/heap/allocation-empty",
				Candidate:      "EmptyAllocationCarrier",
				Input:          "HeapFactCarrier",
				Output:         "HeapFactCarrier",
				Implementation: sourceMethod("Age", "Root", 0),
			},
			{
				Name:           "ClosedAllocationCarryTransform",
				Key:            "transform/heap/allocation-closed",
				Candidate:      "ClosedAllocationCarrier",
				Input:          "HeapFactCarrier",
				Output:         "HeapFactCarrier",
				Implementation: sourceMethod("Age", "Closed", 0),
			},
		},
	}
}

// Source is the stable generator entry point used by callers that name a
// member definition by the axis rather than by its two-form carry family.
func Source() definition.Definition { return AllocationCarry() }
