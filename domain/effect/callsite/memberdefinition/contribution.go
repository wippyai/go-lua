// Package memberdefinition is the generator-only owner source for the two
// exact Call-to-Effect rules' own folds. It is imported by the member
// definition roster and by nothing at runtime, so the callsite package keeps
// its judgment and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	callPackagePath         = "github.com/wippyai/go-lua/domain/call"
	callsitePackagePath     = "github.com/wippyai/go-lua/domain/effect/callsite"
	effectFactorPackagePath = "github.com/wippyai/go-lua/domain/effect/factor"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func judgmentType() definition.GoType {
	return definition.GoType{PackagePath: callsitePackagePath, Name: "Judgment"}
}

// callFactCarrier is the Call fact these folds read. It is named here, in the
// contribution that consumes it, because the reading rule states what it reads
// and the carrier key is Call's own.
func callFactCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "CallFactCarrier", Key: "carrier/call/fact",
		Type: definition.GoType{PackagePath: callPackagePath, Name: "Value"},
	}
}

// mountedCallCarrier is Effect's mounted call site, named on the Call axis
// because the relation below is addressed by one. A relation over Call
// coordinates is Call-axis data whichever rule declares it, and the carrier
// key is Effect's own, so neither axis learns a name the other did not issue.
func mountedCallCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "EffectMountedCallCarrier", Key: "carrier/effect/mounted-call",
		Type: definition.GoType{PackagePath: effectFactorPackagePath, Name: "MountedCall"},
	}
}

func callMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver:        definition.GoType{PackagePath: callPackagePath, Name: receiver},
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func effectSiteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/mounted-call/effect-sites",
	})
}

// effectSites is Call's mounted call directory as an EFFECT-addressed read.
//
// It is a second relation rather than a second input on call/mounted-call/facts
// because a relation's input carrier states which candidate joins it, and that
// one is joined from a Call coordinate - heap's formal freeze reads it that
// way. This one is joined from an Effect mounted call, so it owns its own
// order and declares the correspondence that makes the two enumerable as one:
// both directories are addressed by the mounted occurrence, and neither
// assumes the other numbers its rows alike.
func effectSites() definition.Relation {
	return definition.Relation{
		Name: "MountedEffectCallSites", Key: "call/mounted-call/effect-sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "EffectMountedCallCarrier"}},
		CandidateProvider: effectSiteProvider(),
		CandidateResolver: callMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("effect"), Member: "effect/mounted-call/candidates",
		}},
	}
}

// effectSiteKey addresses the Call cell one mounted site's fact is read at. The
// accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the shared occurrence.
func effectSiteKey() definition.Projection {
	return definition.Projection{
		Name: "MountedEffectCallSiteKey", Key: "call/mounted-call/effect-site-key", Axis: "call",
		Relation:          "MountedEffectCallSites",
		CandidateProvider: effectSiteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callMethod("Key", "CallCoordinate", false, -1),
	}
}

// reducer is the shape both rules share: the mounted Effect call site they are
// indexed by, the one exact Call fact they fold, the Effect fact they publish,
// and the sealed judgment that answers it.
//
// The two rules differ only in which reading of a call target the judgment was
// sealed for, so they name one state type and one method on it, and each seals
// that state through its own constructor.
func reducer(name string, key schema.Key, build string) definition.Reducer {
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
			State:      judgmentType(),
			Build:      definition.GoSymbol{PackagePath: callsitePackagePath, Name: build, ResultIndex: 0},
			StaticAxes: []schema.EntryReference{axisReference("effect"), axisReference("call")},
		},
		Implementation: definition.GoSymbol{
			PackagePath: callsitePackagePath, Name: "Effect",
			Receiver: judgmentType(), ResultIndex: 0,
		},
	}
}

// SelectedContribution is the exact reading's fold: every seed target of the
// call resolves to the effect bindings its operation declares.
func SelectedContribution() definition.Contribution {
	return definition.Contribution{
		Axis:     "effect",
		Rule:     "effect-selected",
		Carriers: []definition.Carrier{callFactCarrier()},
		Reducers: []definition.Reducer{reducer("SelectedCallEffectReducer", "effect/callsite-selected/reducer", "DeriveSelected")},
	}
}

// OpaqueContribution is the opaque reading's fold: the same seed targets
// resolve to the one unknown part the Effect algebra publishes per operation,
// and the call value's opaque alternative joins them.
func OpaqueContribution() definition.Contribution {
	return definition.Contribution{
		Axis:        "effect",
		Rule:        "effect-opaque",
		Carriers:    []definition.Carrier{callFactCarrier(), mountedCallCarrier()},
		Relations:   []definition.Relation{effectSites()},
		Projections: []definition.Projection{effectSiteKey()},
		Reducers:    []definition.Reducer{reducer("OpaqueCallEffectReducer", "effect/callsite-opaque/reducer", "DeriveOpaque")},
	}
}
