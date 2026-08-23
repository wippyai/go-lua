// Package memberdefinition is the generator-only owner source for Value's
// storage-transfer member vocabulary. The runtime value package imports only
// the generated cold catalog; this package is imported by the member
// definition generator, so typed symbol descriptions never become runtime
// callbacks or a second production schema.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath = "github.com/wippyai/go-lua/domain/value"
)

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func valueMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     valuePackagePath,
		Name:            name,
		Receiver:        valueGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// StorageTransfer returns Value's one authored member definition. The member
// generator projects this source into the cold member.Catalog and the
// callback-free typed binding metadata consumed later by composition code.
func StorageTransfer() definition.Definition {
	coordinate := valueGoType("Coordinate")
	storageTransfer := valueGoType("StorageTransfer")
	value := valueGoType("Value")
	allocationResult := valueGoType("AllocationResult")
	freshResultCall := valueGoType("FreshResultCall")
	return definition.Definition{
		Name: "ValueStorageTransfer",
		Axis: "value",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "ValueCoordinateCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: valueMethod("CoordinateIndex", "Schema", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "ValueCoordinateCarrier",
			Fact: "ValueFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: coordinate},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: value},
			{Name: "StorageTransferCarrier", Key: "carrier/value/storage-transfer", Type: storageTransfer},
			{Name: "SourceSeedCarrier", Key: "carrier/value/source-seed", Type: valueGoType("SourceSeed")},
			{Name: "GlobalBootstrapResultCarrier", Key: "carrier/value/global-bootstrap-result", Type: valueGoType("GlobalBootstrapResult")},
			// These are the two owner-issued candidate relationships whose
			// transformed carries write the Value factor. They remain nominal
			// carriers in the cold catalog; no receipt or callback is retained.
			{Name: "AllocationResultCarrier", Key: "carrier/value/allocation-result", Type: allocationResult},
			{Name: "FreshResultCallCarrier", Key: "carrier/value/fresh-result-call", Type: freshResultCall},
		},
		Relations: []definition.Relation{
			{
				Name:              "StorageTransferCandidates",
				Key:               "value/storage-transfer/candidates",
				Subject:           "StorageTransferCarrier",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"},
				CandidateResolver: valueMethod("StorageTransferForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("StorageTransferOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("StorageTransferAt", "Schema", true, 0),
			},
			{
				Name:              "StorageTransferSources",
				Key:               "value/storage-transfer/sources",
				Subject:           "ValueFactCarrier",
				Inputs:            []string{"StorageTransferCarrier"},
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"},
			},
			{
				Name:              "SourceSeeds",
				Key:               "value/source/candidates",
				Subject:           "SourceSeedCarrier",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/source/candidates"},
				CandidateResolver: valueMethod("SourceSeedForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("SourceSeedOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("SourceSeedAt", "Schema", true, 0),
				CandidateCount:    valueMethod("SourceSeedCount", "Schema", true, 0),
				Materialize:       definition.GoSymbol{PackagePath: valuePackagePath, Name: "SourceFact", ResultIndex: 0},
			},
			{
				// The Host global bindings are a Link-global directory: they are
				// addressed by the binding identity alone, and the axis publishes
				// that directory itself rather than an artifact declaring rows for
				// it.
				Name:                "GlobalBootstrapResults",
				Key:                 "value/global-bootstrap/candidates",
				Subject:             "GlobalBootstrapResultCarrier",
				CandidateProvider:   member.RelationRef{Axis: axisReference("value"), Member: "value/global-bootstrap/candidates"},
				CandidateResolver:   valueMethod("GlobalBootstrapResultForID", "Schema", true, 0),
				CandidateOrdinal:    valueMethod("GlobalBootstrapResultOrdinal", "Schema", true, 0),
				CandidateAt:         valueMethod("GlobalBootstrapResultAt", "Schema", true, 0),
				CandidateCount:      valueMethod("GlobalBootstrapResultCount", "Schema", true, 0),
				CandidateIdentityAt: valueMethod("GlobalBootstrapResultIDAt", "Schema", true, 0),
				Materialize:         definition.GoSymbol{PackagePath: valuePackagePath, Name: "GlobalBootstrapFact", ResultIndex: 0},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "StorageTransferSourceKey",
				Key:               "value/storage-transfer/source-key",
				Relation:          "StorageTransferSources",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"},
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Endpoints", "StorageTransfer", false, 0),
			},
			{
				Name:              "StorageTransferTarget",
				Key:               "value/storage-transfer/target",
				Relation:          "StorageTransferCandidates",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"},
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Endpoints", "StorageTransfer", false, 1),
			},
			{
				Name:              "SourceCoordinate",
				Key:               "value/source/coordinate",
				Relation:          "SourceSeeds",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/source/candidates"},
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Result", "SourceSeed", false, 0),
			},
			{
				Name:              "GlobalBootstrapCoordinate",
				Key:               "value/global-bootstrap/coordinate",
				Relation:          "GlobalBootstrapResults",
				CandidateProvider: member.RelationRef{Axis: axisReference("value"), Member: "value/global-bootstrap/candidates"},
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Result", "GlobalBootstrapResult", true, 0),
			},
		},
		CarryTransforms: []definition.CarryTransform{
			{
				Name:           "AllocationCarryTransform",
				Key:            "transform/value/allocation",
				Candidate:      "AllocationResultCarrier",
				Input:          "ValueFactCarrier",
				Output:         "ValueFactCarrier",
				Implementation: valueMethod("Age", "AllocationResult", true, 0),
			},
			{
				Name:           "FreshResultCarryTransform",
				Key:            "transform/value/callresult-freshresult",
				Candidate:      "FreshResultCallCarrier",
				Input:          "ValueFactCarrier",
				Output:         "ValueFactCarrier",
				Implementation: valueMethod("Age", "FreshResultCall", false, 0),
			},
		},
	}
}
