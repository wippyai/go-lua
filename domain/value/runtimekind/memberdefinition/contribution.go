// Package memberdefinition is the generator-only owner source for the
// runtime-kind rule's own fold. It is imported by the member definition roster
// and by nothing at runtime, so the runtimekind package keeps its judgment and
// none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath       = "github.com/wippyai/go-lua/domain/value"
	callPackagePath        = "github.com/wippyai/go-lua/domain/call"
	identityPackagePath    = "github.com/wippyai/go-lua/analysis/identity"
	runtimekindPackagePath = "github.com/wippyai/go-lua/domain/value/runtimekind"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func valueMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: valuePackagePath, Name: name,
		Receiver:        definition.GoType{PackagePath: valuePackagePath, Name: receiver},
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func callMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver:        definition.GoType{PackagePath: callPackagePath, Name: receiver},
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func judgmentType() definition.GoType {
	return definition.GoType{PackagePath: runtimekindPackagePath, Name: "Judgment"}
}

// runtimeKindCarrier is the candidate row this rule folds over: Value's sealed
// interpretation of one strict unary plain call, or of one guarded arm of that
// call's predicate. It is named on both axes below, because the Call-side
// relation this rule reads is joined from one of these rows and states so.
func runtimeKindCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "RuntimeKindCallCarrier", Key: "carrier/value/runtime-kind-call",
		Type: definition.GoType{PackagePath: valuePackagePath, Name: "RuntimeKindCall"},
	}
}

// callFactCarrier is the Call fact this fold reads. It is named here, in the
// contribution that consumes it, because the reading rule states what it reads
// and the carrier key is Call's own.
func callFactCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "CallFactCarrier", Key: "carrier/call/fact",
		Type: definition.GoType{PackagePath: callPackagePath, Name: "Value"},
	}
}

// callOccurrenceCarrier is the occurrence a candidate row names. It is an
// owner-issued identity rather than a coordinate, because the subject it names
// is one this analyzer minted and no dense index of either axis carries it.
func callOccurrenceCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "RuntimeKindCallOccurrenceCarrier", Key: "carrier/value/runtime-kind-call-occurrence",
		Type: definition.GoType{PackagePath: identityPackagePath, Name: "ContentID"},
	}
}

func candidateProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("value"), Member: "value/runtime-kind/candidates",
	})
}

func siteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/runtime-kind/sites",
	})
}

// candidates is Value's runtime-kind directory: one row per mounted occurrence
// whose runtime-kind interpretation Value sealed, drawn from two occurrence
// families - the strict unary plain call and the guarded arm of its predicate.
// The rows are addressed by the shared endpoint table, so this declaration
// introduces no second denominator for them.
func candidates() definition.Relation {
	return definition.Relation{
		Name:              "RuntimeKindCallCandidates",
		Key:               "value/runtime-kind/candidates",
		Subject:           "RuntimeKindCallCarrier",
		CandidateProvider: candidateProvider(),
		CandidateResolver: valueMethod("RuntimeKindCallForMountedOccurrence", "Schema", true, 0),
		CandidateOrdinal:  valueMethod("RuntimeKindCallOrdinal", "Schema", true, 0),
		CandidateAt:       valueMethod("RuntimeKindCallAt", "Schema", true, 0),
	}
}

// subjects is the own-axis read of the value this interpretation observes,
// addressed by the candidate's own coordinate.
func subjects() definition.Relation {
	return definition.Relation{
		Name:              "RuntimeKindSubjects",
		Key:               "value/runtime-kind/subjects",
		Subject:           "ValueFactCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "RuntimeKindCallCarrier"}},
		CandidateProvider: candidateProvider(),
	}
}

// comparisons is the own-axis read of the value the sealed predicate is
// evaluated against. It is a second relation rather than a second projection of
// subjects because a relation's rows are what a read enumerates, and these are
// enumerated at their own coordinate.
func comparisons() definition.Relation {
	return definition.Relation{
		Name:              "RuntimeKindComparisons",
		Key:               "value/runtime-kind/comparisons",
		Subject:           "ValueFactCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "RuntimeKindCallCarrier"}},
		CandidateProvider: candidateProvider(),
	}
}

// sites is Call's mounted call directory as a RUNTIME-KIND-addressed read.
//
// It is a relation of the Call axis because it is addressed by a Call
// coordinate and its key is Call's own; it is a second relation rather than a
// second input on call/mounted-call/facts because a relation's input carrier
// states which candidate joins it, and that one is joined from a Call
// coordinate. This one is joined from a runtime-kind row, so it owns its own
// order and declares the correspondence that makes the two enumerable as one.
// The occurrence they are enumerated under is the one the candidate names,
// because a guarded arm names the call it interprets rather than being it.
func sites() definition.Relation {
	return definition.Relation{
		Name: "RuntimeKindCallSites", Key: "call/runtime-kind/sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "RuntimeKindCallCarrier"}},
		CandidateProvider: siteProvider(),
		CandidateResolver: callMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("value"), Member: "value/runtime-kind/candidates",
		}},
	}
}

// subjectKey addresses the Value cell the observed value's fact is read at.
func subjectKey() definition.Projection {
	return definition.Projection{
		Name:              "RuntimeKindSubjectKey",
		Key:               "value/runtime-kind/subject-key",
		Relation:          "RuntimeKindSubjects",
		CandidateProvider: candidateProvider(),
		Role:              member.Key,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Subject", "RuntimeKindCall", false, -1),
	}
}

// comparisonKey addresses the Value cell the compared value's fact is read at.
func comparisonKey() definition.Projection {
	return definition.Projection{
		Name:              "RuntimeKindComparisonKey",
		Key:               "value/runtime-kind/comparison-key",
		Relation:          "RuntimeKindComparisons",
		CandidateProvider: candidateProvider(),
		Role:              member.Key,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Comparison", "RuntimeKindCall", false, -1),
	}
}

// writeCoordinate is the Value coordinate this rule publishes at: the call
// result for the ordinary transfer, and the narrowed subject for the guarded
// arm. Value issued both while sealing the mounted call, so the rule writes an
// existing coordinate rather than minting one.
func writeCoordinate() definition.Projection {
	return definition.Projection{
		Name:              "RuntimeKindWriteCoordinate",
		Key:               "value/runtime-kind/coordinate",
		Relation:          "RuntimeKindCallCandidates",
		CandidateProvider: candidateProvider(),
		Role:              member.Destination,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("WriteTarget", "RuntimeKindCall", false, -1),
	}
}

// callOccurrence is the occurrence Call's directory enumerates this candidate's
// site under. A plain call row names its own occurrence; a guarded arm names
// the call it interprets, which its own occurrence is not.
func callOccurrence() definition.Projection {
	return definition.Projection{
		Name:              "RuntimeKindCallOccurrence",
		Key:               "value/runtime-kind/call-occurrence",
		Relation:          "RuntimeKindCallCandidates",
		CandidateProvider: candidateProvider(),
		Role:              member.Identity,
		Result:            "RuntimeKindCallOccurrenceCarrier",
		Accessor:          valueMethod("CallOccurrence", "RuntimeKindCall", false, 1),
	}
}

// siteKey addresses the Call cell one runtime-kind site's fact is read at. The
// accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the named occurrence.
func siteKey() definition.Projection {
	return definition.Projection{
		Name: "RuntimeKindCallSiteKey", Key: "call/runtime-kind/site-key", Axis: "call",
		Relation:          "RuntimeKindCallSites",
		CandidateProvider: siteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callMethod("Key", "CallCoordinate", false, -1),
	}
}

// reducer is the shape of this rule's fold: the runtime-kind row it is indexed
// by, the Call fact of the site and the two Value facts it folds, the Value
// fact it publishes, and the sealed judgment that answers it.
func reducer() definition.Reducer {
	return definition.Reducer{
		Name:      "RuntimeKindCallReducer",
		Key:       "value/runtime-kind/reducer",
		Candidate: "RuntimeKindCallCarrier",
		Inputs: []definition.ReducerInput{
			{
				Axis:         axisReference("call"),
				Carrier:      "CallFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			},
			{
				Axis:         axisReference("value"),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			},
			{
				Axis:         axisReference("value"),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			},
		},
		Outputs: []definition.ReducerOutput{{
			Axis:    axisReference("value"),
			Carrier: "ValueFactCarrier",
		}},
		Derivation: definition.ReducerDerivation{
			State:      judgmentType(),
			Build:      definition.GoSymbol{PackagePath: runtimekindPackagePath, Name: "Derive", ResultIndex: 0},
			StaticAxes: []schema.EntryReference{axisReference("value")},
		},
		Implementation: definition.GoSymbol{
			PackagePath: runtimekindPackagePath, Name: "Result",
			Receiver: judgmentType(), ResultIndex: 0,
		},
	}
}

// Contribution is the runtime-kind rule's whole share of the member
// vocabulary: its Value candidate directory and two own-axis reads, the
// Call-side read it is addressed through, and the fold that answers them.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:        "value",
		Rule:        "value-runtime-kind-call",
		Carriers:    []definition.Carrier{runtimeKindCarrier(), callFactCarrier(), callOccurrenceCarrier()},
		Relations:   []definition.Relation{candidates(), subjects(), comparisons(), sites()},
		Projections: []definition.Projection{subjectKey(), comparisonKey(), writeCoordinate(), callOccurrence(), siteKey()},
		Reducers:    []definition.Reducer{reducer()},
	}
}
