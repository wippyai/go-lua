// Package memberdefinition is the generator-only owner source for Value's
// fresh-result rule: its derived route relation, that relation's projections,
// the transition each published row carries, and the fold that answers one
// route. It is imported by the member definition roster and by nothing at
// runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	freshResultPackagePath = "github.com/wippyai/go-lua/domain/value/freshresult"
	valuePackagePath       = "github.com/wippyai/go-lua/domain/value"
	callPackagePath        = "github.com/wippyai/go-lua/domain/call"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func freshResultFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: freshResultPackagePath, Name: name, ResultIndex: 0}
}

func freshResultMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: freshResultPackagePath, Name: name,
		Receiver: goType(freshResultPackagePath, receiver), ResultIndex: resultIndex,
	}
}

func mountedCallProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}
}

// Contribution is the fresh-result rule's own vocabulary on the Value axis.
//
// Value owns and publishes the mounted-call parent and its fresh-result member
// set; this contribution declares only the derived route set over them, the
// transition each published row carries, and the fold. It names Value's and
// Call's rows through the rule Program rather than restating them here.
func Contribution() definition.Contribution {
	value := axisReference("value")
	call := axisReference("call")
	return definition.Contribution{
		Axis: "value",
		Rule: "value-callresult-freshresult",
		Carriers: []definition.Carrier{
			// The two Call carriers this rule's route relation is typed in.
			// They repeat Call's own declaration verbatim; composition refuses
			// a repeat that disagrees.
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: goType(callPackagePath, "CallCoordinate")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "FreshResultRouteTagCarrier", Key: "carrier/value/fresh-result-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
			{
				// The fresh-result route set: the Value coordinates one mounted
				// call publishes a fresh result at. Its inputs are the candidate
				// and that call's own fact, because which arms the call admits
				// is what decides both the destinations and what lands there.
				Name:    "FreshResultRoutes",
				Key:     "value/fresh-result/routes",
				Subject: "FreshResultRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "CallCoordinateCarrier"},
					{Carrier: "CallFactCarrier"},
				},
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Derivation: definition.RelationDerivation{
					State:      goType(freshResultPackagePath, "Plan"),
					Build:      freshResultFunction("DeriveFreshResultRoutes"),
					Count:      freshResultFunction("FreshResultRouteCount"),
					At:         freshResultFunction("FreshResultRouteAt"),
					StaticAxes: []schema.EntryReference{value, call},
				},
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "FreshResultRouteKey",
				Key:               "value/fresh-result/route-key",
				Relation:          "FreshResultRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          freshResultMethod("Coordinates", "Route", 0),
			},
			{
				// A fresh result is written into the result slot it was issued
				// for, so the destination is the same coordinate under its own
				// declared role.
				Name:              "FreshResultRouteDestination",
				Key:               "value/fresh-result/route-destination",
				Relation:          "FreshResultRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          freshResultMethod("Coordinates", "Route", 1),
			},
			{
				Name:              "FreshResultRouteTag",
				Key:               "value/fresh-result/route-tag",
				Relation:          "FreshResultRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Predicate,
				Result:            "FreshResultRouteTagCarrier",
				Accessor:          freshResultMethod("Predicate", "Route", -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "FreshResultReducer",
			Key:       "value/reducer/fresh-result",
			Candidate: "CallCoordinateCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         call,
					Carrier:      "CallFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:         value,
					Carrier:      "ValueFactCarrier",
					Form:         member.ReadFormSelected,
					Multiplicity: member.MultiplicityOne,
					Tag:          "FreshResultRouteTagCarrier",
					Route:        "ValueCoordinateCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{Axis: value, Carrier: "ValueFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State:      goType(freshResultPackagePath, "Judgment"),
				Build:      freshResultFunction("NewJudgment"),
				StaticAxes: []schema.EntryReference{value, call},
			},
			Implementation: freshResultMethod("FreshResultFact", "Judgment", 0),
		}},
	}
}
