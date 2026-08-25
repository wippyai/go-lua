// Package memberdefinition is the generator-only owner source for the
// result-alias rule's own fold: the result-zero candidate directory it is
// indexed by, the Call-side read it is addressed through, the derived alias
// route set it selects over, and the fold that answers them. It is imported by
// the member definition roster and by nothing at runtime, so the resultalias
// package keeps its judgment and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath       = "github.com/wippyai/go-lua/domain/value"
	callPackagePath        = "github.com/wippyai/go-lua/domain/call"
	resultAliasPackagePath = "github.com/wippyai/go-lua/domain/value/resultalias"
	aliasRoutePackagePath  = "github.com/wippyai/go-lua/domain/value/resultalias/aliasroute"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func valueMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: valuePackagePath, Name: name,
		Receiver:        goType(valuePackagePath, receiver),
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func callMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: callPackagePath, Name: name,
		Receiver:        goType(callPackagePath, receiver),
		ReceiverPointer: pointer, ResultIndex: resultIndex,
	}
}

func aliasRouteFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: aliasRoutePackagePath, Name: name, ResultIndex: 0}
}

func aliasRouteMethod(name string) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: aliasRoutePackagePath, Name: name,
		Receiver: goType(aliasRoutePackagePath, "Route"), ResultIndex: -1,
	}
}

func candidateProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("value"), Member: "value/mounted-call-result/candidates",
	})
}

func siteProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("call"), Member: "call/call-result/sites",
	})
}

// MountedCallResultSlotCarrier is the candidate row both call-result folds are
// indexed by: Value's sealed projection of one mounted call's first result
// slot. It is named on both axes below, because the Call-side relation these
// rules read is joined from one of these rows and states so.
func MountedCallResultSlotCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "MountedCallResultSlotCarrier", Key: "carrier/value/mounted-call-result-slot",
		Type: goType(valuePackagePath, "MountedCallResultSlot"),
	}
}

// CallFactCarrier is the Call fact these folds read. It is named here, in the
// contribution that consumes it, because the reading rule states what it reads
// and the carrier key is Call's own.
func CallFactCarrier() definition.Carrier {
	return definition.Carrier{
		Name: "CallFactCarrier", Key: "carrier/call/fact",
		Type: goType(callPackagePath, "Value"),
	}
}

// Candidates is Value's result-zero directory: one row per mounted call whose
// first result Value issued a coordinate for. That is the exact set the
// result-slot requirement issues a placement for, so this declaration
// introduces no second denominator for those rows.
func Candidates() definition.Relation {
	return definition.Relation{
		Name:              "MountedCallResultSlotCandidates",
		Key:               "value/mounted-call-result/candidates",
		Subject:           "MountedCallResultSlotCarrier",
		CandidateProvider: candidateProvider(),
		CandidateResolver: valueMethod("MountedCallResultSlotForMountedOccurrence", "Schema", true, 0),
		CandidateOrdinal:  valueMethod("MountedCallResultSlotOrdinal", "Schema", true, 0),
		CandidateAt:       valueMethod("MountedCallResultSlotAt", "Schema", true, 0),
	}
}

// ResultCoordinate is the call-result Value coordinate these rules publish at.
// Value issued it while sealing the mounted slot, so the rule writes an
// existing coordinate rather than minting one.
func ResultCoordinate() definition.Projection {
	return definition.Projection{
		Name:              "MountedCallResultSlotCoordinate",
		Key:               "value/mounted-call-result/coordinate",
		Relation:          "MountedCallResultSlotCandidates",
		CandidateProvider: candidateProvider(),
		Role:              member.Destination,
		Result:            "ValueCoordinateCarrier",
		Accessor:          valueMethod("Coordinate", "MountedCallResultSlot", false, -1),
	}
}

// Sites is Call's mounted call directory as a RESULT-SLOT-addressed read.
//
// It is a relation of the Call axis because it is addressed by a Call
// coordinate and its key is Call's own; it is a second relation rather than a
// second input on call/mounted-call/facts because a relation's input carrier
// states which candidate joins it, and that one is joined from a Call
// coordinate. This one is joined from a result-slot row, so it owns its own
// order and declares the correspondence that makes the two enumerable as one:
// both directories are addressed by the mounted call occurrence, and neither
// assumes the other numbers its rows alike.
func Sites() definition.Relation {
	return definition.Relation{
		Name: "CallResultSlotSites", Key: "call/call-result/sites", Axis: "call",
		Subject:           "CallCoordinateCarrier",
		Inputs:            []definition.RelationInput{{Carrier: "MountedCallResultSlotCarrier"}},
		CandidateProvider: siteProvider(),
		CandidateResolver: callMethod("CallCoordinateForOccurrence", "Algebra", true, 0),
		CandidateOrdinal:  callMethod("CallCoordinateOrdinal", "Algebra", true, 0),
		CandidateAt:       callMethod("CallCoordinateAt", "Algebra", true, 0),
		Correspondences: []member.RelationRef{{
			Axis: axisReference("value"), Member: "value/mounted-call-result/candidates",
		}},
	}
}

// SiteKey addresses the Call cell one result slot's fact is read at. The
// accessor is a method on Call's own row, because that is the row the
// correspondence resolves through the shared occurrence.
func SiteKey() definition.Projection {
	return definition.Projection{
		Name: "CallResultSlotSiteKey", Key: "call/call-result/site-key", Axis: "call",
		Relation:          "CallResultSlotSites",
		CandidateProvider: siteProvider(),
		Role:              member.Key,
		Result:            "CallKeyCarrier",
		Accessor:          callMethod("Key", "CallCoordinate", false, -1),
	}
}

// aliasRouteCarriers are the member set this rule selects over: one actual the
// selected operations alias the first result to, and the tag it is correlated
// by.
func aliasRouteCarriers() []definition.Carrier {
	return []definition.Carrier{
		{Name: "ResultAliasRouteCarrier", Key: "carrier/value/result-alias-route", Type: goType(aliasRoutePackagePath, "Route")},
		{Name: "ResultAliasRouteTagCarrier", Key: "carrier/value/result-alias-route-tag", Type: definition.GoType{Name: "uint64"}},
	}
}

// aliasRoutes are the actuals this call site aliases its first result to,
// derived once per invocation from the slot and the Call fact read at it. It
// is a relation rather than a table sealed at bind because WHICH actuals a
// call aliases is a property of that call's own dispatch, not of the binding.
func aliasRoutes() definition.Relation {
	return definition.Relation{
		Name:    "ResultAliasRoutes",
		Key:     "value/result-alias/routes",
		Subject: "ResultAliasRouteCarrier",
		Inputs: []definition.RelationInput{
			{Carrier: "MountedCallResultSlotCarrier"},
			{Carrier: "CallFactCarrier"},
		},
		CandidateProvider: candidateProvider(),
		Derivation: definition.RelationDerivation{
			State: goType(aliasRoutePackagePath, "Plan"),
			Build: aliasRouteFunction("Derive"),
			Count: aliasRouteFunction("Count"),
			At:    aliasRouteFunction("At"),
			StaticAxes: []schema.EntryReference{
				axisReference("value"), axisReference("call"), axisReference("pack"),
			},
		},
	}
}

// Contribution is the result-alias rule's whole share of the member
// vocabulary: the result-zero candidate directory and the coordinate it
// publishes at, the Call-side read it is addressed through, the alias route
// set it selects over, and the fold that answers them.
func Contribution() definition.Contribution {
	value := axisReference("value")
	call := axisReference("call")
	return definition.Contribution{
		Axis:      "value",
		Rule:      "value-callresult-resultalias",
		Carriers:  append([]definition.Carrier{MountedCallResultSlotCarrier(), CallFactCarrier()}, aliasRouteCarriers()...),
		Relations: []definition.Relation{Candidates(), Sites(), aliasRoutes()},
		Projections: []definition.Projection{
			ResultCoordinate(),
			SiteKey(),
			{
				Name: "ResultAliasRouteKey", Key: "value/result-alias/route-key",
				Relation: "ResultAliasRoutes", CandidateProvider: candidateProvider(),
				Role: member.Key, Result: "ValueCoordinateCarrier", Accessor: aliasRouteMethod("Coordinate"),
			},
			{
				Name: "ResultAliasRouteTag", Key: "value/result-alias/route-tag",
				Relation: "ResultAliasRoutes", CandidateProvider: candidateProvider(),
				Role: member.Predicate, Result: "ResultAliasRouteTagCarrier", Accessor: aliasRouteMethod("Predicate"),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "ResultAliasReducer",
			Key:       "value/result-alias/reducer",
			Candidate: "MountedCallResultSlotCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         call,
					Carrier:      "CallFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:    value,
					Carrier: "ValueFactCarrier",
					Form:    member.ReadFormSelected,
					// Many-valued: this fold is handed the whole selection and
					// concludes once over it. The tag carrier is still named,
					// because which carrier names a member is the joined axis's
					// statement either way - a delivery this wide carries the
					// tags inside its cells rather than as an argument beside
					// them.
					Multiplicity: member.MultiplicityMany,
					Tag:          "ResultAliasRouteTagCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{Axis: value, Carrier: "ValueFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State: goType(resultAliasPackagePath, "Judgment"),
				Build: definition.GoSymbol{PackagePath: resultAliasPackagePath, Name: "Derive", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{
					axisReference("value"), axisReference("call"), axisReference("pack"),
				},
			},
			Implementation: definition.GoSymbol{
				PackagePath: resultAliasPackagePath, Name: "Result",
				Receiver: goType(resultAliasPackagePath, "Judgment"), ResultIndex: 0,
			},
		}},
	}
}
