// Package memberdefinition is the generator-only owner source for Pack's
// source member vocabulary. The runtime pack package imports only the
// generated cold catalog; this package is imported by the member definition
// generator.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	packPackagePath = "github.com/wippyai/go-lua/domain/pack"
)

func packGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: packPackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func packMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     packPackagePath,
		Name:            name,
		Receiver:        packGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// Source returns Pack's authored member definition for the zero-input source
// Rule. The member generator projects this source into the cold member.Catalog
// and the callback-free typed binding metadata consumed later by composition.
func Source() definition.Definition {
	root := packGoType("Root")
	value := packGoType("Value")
	source := packGoType("Source")
	return definition.Definition{
		Name:       "PackSource",
		Axis:       "pack",
		ImportPath: "github.com/wippyai/go-lua/domain/pack",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "RootCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: packMethod("RootIndex", "Schema", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "RootCarrier",
			Fact: "FactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "RootCarrier", Key: "carrier/pack/root", Type: root},
			{Name: "FactCarrier", Key: "carrier/pack/fact", Type: value},
			{Name: "SourceCarrier", Key: "carrier/pack/source", Type: source},
		},
		Relations: []definition.Relation{
			{
				Name:              "SourceSeeds",
				Key:               "pack/source/candidates",
				Subject:           "SourceCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("pack"), Member: "pack/source/candidates"}),
				CandidateResolver: packMethod("SourceForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  packMethod("SourceOrdinal", "Schema", true, 0),
				CandidateAt:       packMethod("SourceAt", "Schema", true, 0),
				CandidateCount:    packMethod("SourceCount", "Schema", true, 0),
				Materialize:       definition.GoSymbol{PackagePath: packPackagePath, Name: "SourceFact", ResultIndex: 0},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "SourceCoordinate",
				Key:               "pack/source/coordinate",
				Relation:          "SourceSeeds",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("pack"), Member: "pack/source/candidates"}),
				Role:              member.Destination,
				Result:            "RootCarrier",
				Accessor:          packMethod("Result", "Source", false, 0),
			},
		},
	}
}
