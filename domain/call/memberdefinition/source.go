// Package memberdefinition is the generator-only owner source for Call's
// mounted-call member vocabulary. The runtime call package imports only the
// generated cold catalog; this package is imported by the member definition
// generator, so typed symbol descriptions never become runtime callbacks or a
// second production schema.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	callPackagePath = "github.com/wippyai/go-lua/domain/call"
)

func callGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: callPackagePath, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func callMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     callPackagePath,
		Name:            name,
		Receiver:        callGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// MountedCall returns Call's one authored member definition: the sealed
// occurrence-to-coordinate projection domain/call/call_coordinate.go already
// publishes, named as the axis's member catalog so a Rule Program can declare
// a mounted call occurrence as a candidate.
func MountedCall() definition.Definition {
	key := callGoType("Key")
	value := callGoType("Value")
	coordinate := callGoType("CallCoordinate")
	return definition.Definition{
		Name:       "CallMountedCall",
		Axis:       "call",
		ImportPath: "github.com/wippyai/go-lua/domain/call",
		Binding: definition.Binding{
			Key: definition.KeyNormalization{
				Carrier:    "CallKeyCarrier",
				Dense:      builtinGoType("uint32"),
				Normalizer: callMethod("DenseKeyIndex", "Algebra", true, 0),
			},
		},
		Signature: definition.Signature{
			Key:  "CallKeyCarrier",
			Fact: "CallFactCarrier",
		},
		Carriers: []definition.Carrier{
			{Name: "CallKeyCarrier", Key: "carrier/call/key", Type: key},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: value},
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: coordinate},
			{Name: "CallTargetCarrier", Key: "carrier/call/target", Type: callGoType("Target")},
		},
		// How a Call value decomposes is Call's own answer, so the two
		// sequences a consumer reads out of one are declared here once rather
		// than carried as a symbol pair by every rule that reads them.
		Enumerations: []definition.Enumeration{
			{
				// The alternatives a call value names.
				Name: "KnownTargets", Over: "CallFactCarrier", Item: "CallTargetCarrier",
				Count: callMethod("KnownTargetCount", "Value", false, -1),
				At:    callMethod("KnownTargetAt", "Value", false, 0),
			},
			{
				// Over nothing: the whole executable body directory, which is
				// what a value that named no alternatives widens to.
				Name: "BodyTargets", Item: "CallTargetCarrier",
				Count: callMethod("BodyTargetCount", "Algebra", true, -1),
				At:    callMethod("BodyTargetAt", "Algebra", true, 0),
			},
		},
		Relations: []definition.Relation{
			{
				// The candidates are the mounted call occurrences; the
				// projection is already sealed content-addressed rows, so this
				// relation carries no source materialization and no global
				// occurrence directory - it is mount-qualified, not a Link-wide
				// directory.
				Name:              "MountedCallCandidates",
				Key:               "call/mounted-call/candidates",
				Subject:           "CallCoordinateCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}),
				CandidateResolver: callMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
				CandidateOrdinal:  callMethod("CallCoordinateOrdinal", "Algebra", true, 0),
				CandidateAt:       callMethod("CallCoordinateAt", "Algebra", true, 0),
			},
			{
				// The dispatch fact relation is indexed by the same mounted Call
				// candidate that owns its exact destination key. It declares the
				// foreign Call read a Rule Program consumes; it does not materialize
				// or duplicate a runtime Call value.
				Name:              "MountedCallFacts",
				Key:               "call/mounted-call/facts",
				Subject:           "CallFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "MountedCallCoordinate",
				Key:               "call/mounted-call/coordinate",
				Relation:          "MountedCallCandidates",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}),
				Role:              member.Destination,
				Result:            "CallKeyCarrier",
				Accessor:          callMethod("Key", "CallCoordinate", false, -1),
			},
			{
				Name:              "MountedCallFactKey",
				Key:               "call/mounted-call/fact-key",
				Relation:          "MountedCallFacts",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}),
				Role:              member.Key,
				Result:            "CallKeyCarrier",
				Accessor:          callMethod("Key", "CallCoordinate", false, -1),
			},
		},
	}
}

// Source is the stable generator entry point used by callers that name a
// member definition by the axis rather than by its one candidate family.
func Source() definition.Definition { return MountedCall() }
