// Package memberdefinition is the generator-only owner source for Effect's
// mounted-call member vocabulary. The runtime effect package imports only the
// generated cold catalog; this package is imported by the member definition
// generator, so typed symbol descriptions never become runtime callbacks or a
// second production schema.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	effectPackagePath = "github.com/wippyai/go-lua/domain/effect/factor"
	callPackagePath   = "github.com/wippyai/go-lua/domain/call"
)

func effectGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: effectPackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func effectMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     effectPackagePath,
		Name:            name,
		Receiver:        effectGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// MountedCall returns Effect's one authored member definition: the canonical
// mounted-call directory domain/effect/factor/mounted_call.go already
// publishes, named as the axis's member catalog so a Rule Program can declare
// a mounted call site as a candidate and project its own Root coordinate.
func MountedCall() definition.Definition {
	key := effectGoType("Root")
	value := effectGoType("Value")
	mounted := effectGoType("MountedCall")
	return definition.Definition{
		Name:       "EffectMountedCall",
		Axis:       "effect",
		ImportPath: "github.com/wippyai/go-lua/domain/effect",
		// The relation owner binds *factor.Algebra, so it must import
		// domain/effect/factor - which imports domain/static. Bare
		// domain/effect does not, and internal/testfixture (which
		// domain/static's own tests reach through) imports bare
		// domain/effect for its Label/ParamRef/Row vocabulary. Generating
		// the relation owner into domain/effect/owner instead - the package
		// that already binds *factor.Algebra for the hot owner - keeps that
		// import out of the package testfixture pulls in, so it never
		// closes back on domain/static's test.
		RelationsPackage: "owner",
		RelationsPath:    "owner/generated_relation_owner.go",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "EffectKeyCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: effectMethod("DenseKeyIndex", "Algebra", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "EffectKeyCarrier",
			Fact: "EffectFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "EffectKeyCarrier", Key: "carrier/effect/key", Type: key, Capability: carrier.Equatable},
			{Name: "EffectFactCarrier", Key: "carrier/effect/fact", Type: value, Capability: carrier.Ascending},
			{Name: "EffectMountedCallCarrier", Key: "carrier/effect/mounted-call", Type: mounted, Capability: carrier.DecodeOnly},
			{Name: "PublicationSourceCarrier", Key: "carrier/effect/mounted-publication-source", Type: effectGoType("PublicationSubject"), Capability: carrier.DecodeOnly},
			{Name: "PublicationSourceTagCarrier", Key: "carrier/effect/mounted-publication-source-tag", Type: builtinGoType("uint64"), Capability: carrier.DecodeOnly},
		},
		CarrierRefs: []definition.CarrierReference{{
			Name: "CallFactCarrier", Key: "carrier/call/fact",
			Ref:  carrier.Ref{Owner: axisReference("call"), Carrier: "carrier/call/fact"},
			Type: definition.GoType{PackagePath: callPackagePath, Name: "Value"},
		}},
		Relations: []definition.Relation{
			{
				// The candidates are the mounted call sites Effect already
				// enumerates on its own canonical directory; this relation
				// carries no source materialization and no second occurrence
				// directory - MountedCallForOccurrence is the sole exported
				// inverse domain/effect/factor already composes.
				Name:              "MountedEffectCallCandidates",
				Key:               "effect/mounted-call/candidates",
				Subject:           "EffectMountedCallCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("effect"), Member: "effect/mounted-call/candidates"}),
				CandidateResolver: effectMethod("MountedCallForOccurrence", "Algebra", true, 0),
				CandidateOrdinal:  effectMethod("MountedCallOrdinal", "Algebra", true, 0),
				CandidateAt:       effectMethod("MountedCallAt", "Algebra", true, 0),
			},
			{
				// Publication subject members are Effect-owned rows reached from
				// the canonical mounted-call candidate and authenticated Call fact.
				Name: "PublicationSources", Key: "effect/mounted-publication/sources",
				Subject: "PublicationSourceCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "EffectMountedCallCarrier"},
					{Carrier: "CallFactCarrier"},
				},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("effect"), Member: "effect/mounted-call/candidates"}),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "MountedEffectCallCoordinate",
				Key:               "effect/mounted-call/coordinate",
				Relation:          "MountedEffectCallCandidates",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("effect"), Member: "effect/mounted-call/candidates"}),
				Role:              member.Destination,
				Result:            "EffectKeyCarrier",
				Accessor:          effectMethod("Root", "MountedCall", false, -1),
			},
			{
				Name: "PublicationSourceTag", Key: "effect/mounted-publication/source-tag",
				Relation: "PublicationSources", Role: member.Predicate,
				Result:            "PublicationSourceTagCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("effect"), Member: "effect/mounted-call/candidates"}),
				Accessor:          effectMethod("Predicate", "PublicationSubject", false, -1),
			},
		},
		Selections: []definition.Selection{{
			Name: "PublicationSourceSelection", Key: "effect/publication-escape/source-selection",
			Relation: "PublicationSources", Tag: "PublicationSourceTag",
			Implementation: effectMethod("PublicationSubjectAt", "Algebra", true, 0),
		}},
	}
}

// Source is the stable generator entry point used by callers that name a
// member definition by the axis rather than by its one candidate family.
func Source() definition.Definition { return MountedCall() }
