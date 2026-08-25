// Package memberdefinition is the generator-only owner source for Static's
// typed-fact transfer member vocabulary. The runtime static package imports
// only the generated cold catalog; this package is imported by the member
// definition generator.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	staticPackagePath = "github.com/wippyai/go-lua/domain/static"
	valuePackagePath  = "github.com/wippyai/go-lua/domain/value"
)

func staticGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: staticPackagePath, Name: name}
}

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

// storageTransferProvider is Value's storage-transfer directory: the earliest
// owner of which transfers exist and in what order.
func storageTransferProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("value"), Member: "value/storage-transfer/candidates"}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// TypeFactTransfer returns Static's authored member definition for the
// identity TypeFact copy along Value's sealed StorageTransfer. Candidate and
// destination stay on Value's catalog; this definition owns the join, key,
// and reducer used to read and write Static's factor at those coordinates.
func TypeFactTransfer() definition.Definition {
	coordinate := valueGoType("Coordinate")
	storageTransfer := valueGoType("StorageTransfer")
	typeFact := staticGoType("TypeFact")
	return definition.Definition{
		Name:       "StaticTypeFactTransfer",
		Axis:       "static-type",
		ImportPath: "github.com/wippyai/go-lua/domain/static",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "CoordinateCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: valueMethod("CoordinateIndex", "Schema", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "CoordinateCarrier",
			Fact: "TypeFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "CoordinateCarrier", Key: "carrier/value/coordinate", Type: coordinate},
			{Name: "TypeFactCarrier", Key: "carrier/static-type/fact", Type: typeFact},
			{Name: "StorageTransferCarrier", Key: "carrier/value/storage-transfer", Type: storageTransfer},
		},
		Relations: []definition.Relation{
			{
				// Static's own dense order over the transfers Value owns. It
				// resolves through Value's directory, so the two enumerations
				// are one, and the correspondence says so: a rule keyed by
				// Value's candidate addresses these rows with that ordinal.
				//
				// The copy exists because a foreign-provided relation's
				// projections are not emitted - the generator has no way to
				// reach the foreign owner for the subject an ordinal stands
				// for - so Static cannot yet simply name Value's directory.
				Name:              "TypeFactTransfers",
				Key:               "static-type/storage-transfer/candidates",
				Subject:           "StorageTransferCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("static-type"), Member: "static-type/storage-transfer/candidates"}),
				CandidateResolver: valueMethod("StorageTransferForArtifactOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("StorageTransferOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("StorageTransferAt", "Schema", true, 0),
				Correspondences:   []member.RelationRef{storageTransferProvider()},
			},
			{
				Name:              "TypeFactSources",
				Key:               "static-type/storage-transfer/sources",
				Subject:           "TypeFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "StorageTransferCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("static-type"), Member: "static-type/storage-transfer/candidates"}),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "TypeFactSourceKey",
				Key:               "static-type/storage-transfer/source-key",
				Relation:          "TypeFactSources",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("static-type"), Member: "static-type/storage-transfer/candidates"}),
				Role:              member.Key,
				Result:            "CoordinateCarrier",
				Accessor:          valueMethod("Endpoints", "StorageTransfer", false, 0),
			},
		},
	}
}
