// Package memberdefinition is the generator-only owner source for the
// module-load rule's own fold. It is imported by the member definition roster
// and by nothing at runtime, so the moduleload package keeps its judgment and
// none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath      = "github.com/wippyai/go-lua/domain/value"
	callPackagePath       = "github.com/wippyai/go-lua/domain/call"
	moduleloadPackagePath = "github.com/wippyai/go-lua/domain/value/moduleload"
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
	return definition.GoType{PackagePath: moduleloadPackagePath, Name: "Judgment"}
}

// moduleLoadCarrier is the candidate row this rule folds over: Value's sealed
// interpretation of one scoped require call. It is named on both axes below,
// because the Call-side relation this rule reads is joined from one of these
// rows and states so.
func moduleLoadCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "ModuleLoadCallCarrier", Key: "carrier/value/module-load-call",
		Type: definition.GoType{PackagePath: valuePackagePath, Name: "ModuleLoadCall"},
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

func candidateProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("value"), Member: "value/module-load/candidates",
	})
}

func siteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/module-load/sites",
	})
}

// candidates is Value's module-load directory: one row per mounted scoped
// require call whose result Value already issued a coordinate for. The rows
// are addressed by the shared endpoint table, so this declaration introduces
// no second denominator for them.
func candidates() definition.Relation {
	return definition.Relation{
		Name:              "ModuleLoadCallCandidates",
		Key:               "value/module-load/candidates",
		Subject:           "ModuleLoadCallCarrier",
		CandidateProvider: candidateProvider(),
		CandidateResolver: valueMethod("ModuleLoadCallForMountedOccurrence", "Schema", true, 0),
		CandidateOrdinal:  valueMethod("ModuleLoadCallOrdinal", "Schema", true, 0),
		CandidateAt:       valueMethod("ModuleLoadCallAt", "Schema", true, 0),
	}
}

// arguments is the own-axis read: the Value fact of the single actual the
// scoped loader is applied to, addressed by the candidate's own coordinate.
func arguments() definition.Relation {
	return definition.Relation{
		Name:              "ModuleLoadArguments",
		Key:               "value/module-load/arguments",
		Subject:           "ValueFactCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "ModuleLoadCallCarrier"}},
		CandidateProvider: candidateProvider(),
	}
}

// sites is Call's mounted call directory as a MODULE-LOAD-addressed read.
//
// It is a relation of the Call axis because it is addressed by a Call
// coordinate and its key is Call's own; it is a second relation rather than a
// second input on call/mounted-call/facts because a relation's input carrier
// states which candidate joins it, and that one is joined from a Call
// coordinate. This one is joined from a module-load row, so it owns its own
// order and declares the correspondence that makes the two enumerable as one:
// both directories are addressed by the mounted call occurrence, and neither
// assumes the other numbers its rows alike.
func sites() definition.Relation {
	return definition.Relation{
		Name: "ModuleLoadCallSites", Key: "call/module-load/sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "ModuleLoadCallCarrier"}},
		CandidateProvider: siteProvider(),
		CandidateResolver: callMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("value"), Member: "value/module-load/candidates",
		}},
	}
}

// argumentKey addresses the Value cell the actual's fact is read at.
func argumentKey() definition.Projection {
	return definition.Projection{
		Name:              "ModuleLoadArgumentKey",
		Key:               "value/module-load/argument-key",
		Relation:          "ModuleLoadArguments",
		CandidateProvider: candidateProvider(),
		Role:              member.Key,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Argument", "ModuleLoadCall", false, -1),
	}
}

// resultCoordinate is the call-result Value coordinate this rule publishes at.
// Value issued it while sealing the mounted call, so the rule writes an
// existing coordinate rather than minting one.
func resultCoordinate() definition.Projection {
	return definition.Projection{
		Name:              "ModuleLoadResultCoordinate",
		Key:               "value/module-load/coordinate",
		Relation:          "ModuleLoadCallCandidates",
		CandidateProvider: candidateProvider(),
		Role:              member.Destination,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Result", "ModuleLoadCall", false, -1),
	}
}

// siteKey addresses the Call cell one module-load site's fact is read at. The
// accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the shared occurrence.
func siteKey() definition.Projection {
	return definition.Projection{
		Name: "ModuleLoadCallSiteKey", Key: "call/module-load/site-key", Axis: "call",
		Relation:          "ModuleLoadCallSites",
		CandidateProvider: siteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callMethod("Key", "CallCoordinate", false, -1),
	}
}

// reducer is the shape of this rule's fold: the module-load row it is indexed
// by, the Value fact of the actual and the Call fact of the site it folds, the
// Value fact it publishes, and the sealed judgment that answers it.
func reducer() definition.Reducer {
	return definition.Reducer{
		Name:      "ModuleLoadCallReducer",
		Key:       "value/module-load/reducer",
		Candidate: "ModuleLoadCallCarrier",
		Inputs: []definition.ReducerInput{
			{
				Axis:         axisReference("value"),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			},
			{
				Axis:         axisReference("call"),
				Carrier:      "CallFactCarrier",
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
			Build:      definition.GoSymbol{PackagePath: moduleloadPackagePath, Name: "Derive", ResultIndex: 0},
			StaticAxes: []schema.EntryReference{axisReference("value")},
		},
		Implementation: definition.GoSymbol{
			PackagePath: moduleloadPackagePath, Name: "Result",
			Receiver: judgmentType(), ResultIndex: 0,
		},
	}
}

// Contribution is the module-load rule's whole share of the member vocabulary:
// its Value candidate directory and own-axis read, the Call-side read it is
// addressed through, and the fold that answers them.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:        "value",
		Rule:        "value-callresult-moduleload",
		Carriers:    []definition.Carrier{moduleLoadCarrier(), callFactCarrier()},
		Relations:   []definition.Relation{candidates(), arguments(), sites()},
		Projections: []definition.Projection{argumentKey(), resultCoordinate(), siteKey()},
		Reducers:    []definition.Reducer{reducer()},
	}
}
