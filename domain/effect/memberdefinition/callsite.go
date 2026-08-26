package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// This file declares the rows the three call-site rules are addressed
// through. Each of them - the exact reading, the opaque reading and the
// interprocedural one - is indexed by the same mounted Effect call site, reads
// the same Call fact, and folds with the same sealed judgment shape; only
// which reading of a call target that judgment was sealed for differs.
// Declaring those rows once, in the axis owner's own generator source, is what
// keeps three rules from carrying three copies of a vocabulary that is none of
// theirs.

const (
	callPackagePath         = "github.com/wippyai/go-lua/domain/call"
	callsitePackagePath     = "github.com/wippyai/go-lua/domain/effect/callsite"
	effectFactorPackagePath = "github.com/wippyai/go-lua/domain/effect/factor"
)

// CallsiteJudgmentType is the sealed state all three call-site folds answer
// through.
func CallsiteJudgmentType() definition.GoType {
	return definition.GoType{PackagePath: callsitePackagePath, Name: "Judgment"}
}

// CallFactCarrier is the Call fact these folds read. The carrier key is Call's
// own, so neither axis learns a name the other did not issue.
func CallFactCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "CallFactCarrier", Key: "carrier/call/fact",
		Type: definition.GoType{PackagePath: callPackagePath, Name: "Value"},
	}
}

// MountedCallCarrier is Effect's mounted call site, named on the Call axis
// because the relation below is addressed by one. A relation over Call
// coordinates is Call-axis data whichever rule declares it, and the carrier
// key is Effect's own.
func MountedCallCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "EffectMountedCallCarrier", Key: "carrier/effect/mounted-call",
		Type: definition.GoType{PackagePath: effectFactorPackagePath, Name: "MountedCall"},
	}
}

func callsiteMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver:        definition.GoType{PackagePath: callPackagePath, Name: receiver},
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

// EffectSiteProvider is the Call-side directory the corresponded site relation
// below owns its order in.
func EffectSiteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/mounted-call/effect-sites",
	})
}

// EffectSites is Call's mounted call directory as an EFFECT-addressed read.
//
// It is a second relation rather than a second input on call/mounted-call/facts
// because a relation's input carrier states which candidate joins it, and that
// one is joined from a Call coordinate - heap's formal freeze reads it that
// way. This one is joined from an Effect mounted call, so it owns its own
// order and declares the correspondence that makes the two enumerable as one:
// both directories are addressed by the mounted occurrence, and neither
// assumes the other numbers its rows alike.
func EffectSites() definition.Relation {
	return definition.Relation{
		Name: "MountedEffectCallSites", Key: "call/mounted-call/effect-sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "EffectMountedCallCarrier"}},
		CandidateProvider: EffectSiteProvider(),
		CandidateResolver: callsiteMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callsiteMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callsiteMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("effect"), Member: "effect/mounted-call/candidates",
		}},
	}
}

// EffectSiteKey addresses the Call cell one mounted site's fact is read at. The
// accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the shared occurrence.
func EffectSiteKey() definition.Projection {
	return definition.Projection{
		Name: "MountedEffectCallSiteKey", Key: "call/mounted-call/effect-site-key", Axis: "call",
		Relation:          "MountedEffectCallSites",
		CandidateProvider: EffectSiteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callsiteMethod("Key", "CallCoordinate", false, -1),
	}
}

// CallsiteReducer is the shape the exact and opaque readings share: the
// mounted Effect call site they are indexed by, the one exact Call fact they
// fold, the Effect fact they publish, and the sealed judgment that answers it.
//
// The two rules differ only in which reading of a call target the judgment was
// sealed for, so they name one state type and one method on it, and each seals
// that state through its own constructor.
func CallsiteReducer(name string, key schema.Key, build string) definition.Reducer {
	return definition.Reducer{
		Name:      name,
		Key:       key,
		Candidate: "EffectMountedCallCarrier",
		Inputs: []definition.ReducerInput{{
			Axis:         axisReference("call"),
			Carrier:      "CallFactCarrier",
			Form:         member.ReadFormExact,
			Multiplicity: member.MultiplicityOne,
		}},
		Outputs: []definition.ReducerOutput{{
			Axis:    axisReference("effect"),
			Carrier: "EffectFactCarrier",
		}},
		Derivation: definition.ReducerDerivation{
			State:      CallsiteJudgmentType(),
			Build:      definition.GoSymbol{PackagePath: callsitePackagePath, Name: build, ResultIndex: 0},
			StaticAxes: []schema.EntryReference{axisReference("effect"), axisReference("call")},
		},
		Implementation: definition.GoSymbol{
			PackagePath: callsitePackagePath, Name: "Effect",
			Receiver: CallsiteJudgmentType(), ResultIndex: 0,
		},
	}
}
