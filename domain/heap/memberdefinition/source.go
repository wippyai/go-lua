// Package memberdefinition is the generator-only owner source for Heap's
// transformed-carry member vocabulary. The runtime heap package imports only
// the generated cold catalog; this package carries no executable handle or
// runtime callback.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	heapPackagePath     = "github.com/wippyai/go-lua/domain/heap"
	sourcePackagePath   = "github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	identityPackagePath = "github.com/wippyai/go-lua/analysis/identity"
	summaryPackagePath  = "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
)

func heapGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: heapPackagePath, Name: name}
}

func sourceGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: sourcePackagePath, Name: name}
}

func identityGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: identityPackagePath, Name: name}
}

func summaryGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: summaryPackagePath, Name: name}
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

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// AllocationCarry returns Heap's two distinct allocation-form carry members.
// Both candidate methods apply Heap.Schema.Age, but the member keys and
// candidate relationships remain separate semantic identities for empty and
// closed allocation rules.
func AllocationCarry() definition.Definition {
	return definition.Definition{
		Name:       "HeapAllocationCarry",
		Axis:       "heap",
		ImportPath: "github.com/wippyai/go-lua/domain/heap",
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "HeapKeyCarrier",
			Dense:      builtinGoType("uint32"),
			Normalizer: heapMethod("DenseKeyIndex", "Schema", false, 0),
		}},
		Signature: definition.Signature{Key: "HeapKeyCarrier", Fact: "HeapFactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "HeapKeyCarrier", Key: "carrier/heap/key", Type: heapGoType("Key"), Capability: carrier.Equatable},
			{Name: "HeapFactCarrier", Key: "carrier/heap/fact", Type: heapGoType("Value"), Capability: carrier.Ascending},
			{Name: "AllocationIDCarrier", Key: "carrier/heap/allocation-id", Type: identityGoType("ContentID"), Capability: carrier.Equatable},
			{Name: "AllocationSourceCarrier", Key: "carrier/heap/allocation-source", Type: summaryGoType("Source"), Capability: carrier.DecodeOnly},
			{Name: "EmptyAllocationCarrier", Key: "carrier/heap/allocation-empty", Type: sourceGoType("Root"), Capability: carrier.DecodeOnly},
			{Name: "ClosedAllocationCarrier", Key: "carrier/heap/allocation-closed", Type: sourceGoType("Closed"), Capability: carrier.DecodeOnly},
		},
		Relations: []definition.Relation{
			{
				Name:              "IngressSeeds",
				Key:               "heap/ingress/candidates",
				Subject:           "HeapKeyCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/ingress/candidates"}),
				CandidateResolver: heapMethod("AllocationRootForMountedOccurrence", "Schema", false, 0),
				CandidateOrdinal:  heapMethod("AllocationRootOrdinal", "Schema", false, 0),
				CandidateAt:       heapMethod("AllocationRootAt", "Schema", false, 0),
				CandidateCount:    heapMethod("AllocationRootCount", "Schema", false, 0),
				Materialize:       definition.GoSymbol{PackagePath: heapPackagePath, Name: "IngressFact", ResultIndex: 0},
			},
			{
				// Bootstrap roots are a Link-global directory: the Host binding
				// identity addresses one root on its own, and Heap publishes that
				// directory rather than an artifact declaring rows for it.
				Name:                "BootRoots",
				Key:                 "heap/boot/candidates",
				Subject:             "HeapKeyCarrier",
				CandidateProvider:   member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/boot/candidates"}),
				CandidateResolver:   heapMethod("KeyForBootID", "Schema", false, 0),
				CandidateOrdinal:    heapMethod("BootRootOrdinal", "Schema", false, 0),
				CandidateAt:         heapMethod("BootRootAt", "Schema", false, 0),
				CandidateCount:      heapMethod("BootCount", "Schema", false, 0),
				CandidateIdentityAt: heapMethod("BootIDAt", "Schema", false, 0),
				Materialize:         definition.GoSymbol{PackagePath: heapPackagePath, Name: "BootFact", ResultIndex: 0},
			},
			{
				// The two constructor-form directories are the heap axis's own
				// dense global candidate sets. A form directory is published
				// here, beside the roots it partitions, because an allocation
				// occurrence resolves to one Link-wide ordinal that every
				// reader shares; a per-mount substitution map has no row for
				// the mount it is not and cannot state that ordinal.
				Name:              "ClosedAllocations",
				Key:               "heap/closed-allocation/candidates",
				Subject:           "HeapKeyCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
				CandidateResolver: heapMethod("ClosedAllocationForMountedOccurrence", "Schema", false, 0),
				CandidateOrdinal:  heapMethod("ClosedAllocationOrdinal", "Schema", false, 0),
				CandidateAt:       heapMethod("ClosedAllocationAt", "Schema", false, 0),
			},
			{
				Name:              "EmptyAllocations",
				Key:               "heap/empty-allocation/candidates",
				Subject:           "HeapKeyCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/empty-allocation/candidates"}),
				CandidateResolver: heapMethod("EmptyAllocationForMountedOccurrence", "Schema", false, 0),
				CandidateOrdinal:  heapMethod("EmptyAllocationOrdinal", "Schema", false, 0),
				CandidateAt:       heapMethod("EmptyAllocationAt", "Schema", false, 0),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "IngressCoordinate",
				Key:               "heap/ingress/coordinate",
				Relation:          "IngressSeeds",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/ingress/candidates"}),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("Ingress", "Key", false, 0),
			},
			{
				Name:              "BootCoordinate",
				Key:               "heap/boot/coordinate",
				Relation:          "BootRoots",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/boot/candidates"}),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("Boot", "Key", false, 0),
			},
			{
				// A constructor writes the very root it is a candidate of, so
				// the destination projection is the candidate itself, refused
				// on any key the relation does not contain.
				Name:              "ClosedAllocationCoordinate",
				Key:               "heap/closed-allocation/coordinate",
				Relation:          "ClosedAllocations",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("ClosedAllocation", "Key", false, -1),
			},
			{
				Name:              "EmptyAllocationCoordinate",
				Key:               "heap/empty-allocation/coordinate",
				Relation:          "EmptyAllocations",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/empty-allocation/candidates"}),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("EmptyAllocation", "Key", false, -1),
			},
		},
		CarryTransforms: []definition.CarryTransform{
			// Both forms carry with the same owner-issued transition, applied
			// to the allocation coordinate rather than to a constructor
			// descriptor: the candidate a rule carries is the subject of the
			// relation it draws candidates from, and a descriptor that lives
			// beside the fold is no relation's subject. The two keys remain
			// separate semantic identities for the empty and closed rules.
			{
				Name:           "EmptyAllocationCarryTransform",
				Key:            "transform/heap/allocation-empty",
				Candidate:      "HeapKeyCarrier",
				Input:          "HeapFactCarrier",
				Output:         "HeapFactCarrier",
				Implementation: heapMethod("Age", "Key", false, 0),
			},
			{
				Name:           "ClosedAllocationCarryTransform",
				Key:            "transform/heap/allocation-closed",
				Candidate:      "HeapKeyCarrier",
				Input:          "HeapFactCarrier",
				Output:         "HeapFactCarrier",
				Implementation: heapMethod("Age", "Key", false, 0),
			},
		},
	}
}

// Source is the stable generator entry point used by callers that name a
// member definition by the axis rather than by its two-form carry family.
func Source() definition.Definition { return AllocationCarry() }
