// Package memberdefinition is the generator-only owner source for the
// body-result rule's own fold: the derived return route set it selects over
// and the fold that answers it. The rows it shares with the other call-result
// transfer - the result-zero directory, the coordinate it publishes at, and
// the Call-side read it is addressed through - are the axis owner's own and
// are named from there. It is imported by the member definition roster and by
// nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	valuebase "github.com/wippyai/go-lua/domain/value/memberdefinition"
)

const (
	bodyResultPackagePath  = "github.com/wippyai/go-lua/domain/value/bodyresult"
	returnRoutePackagePath = "github.com/wippyai/go-lua/domain/value/bodyresult/returnroute"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func returnRouteFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: returnRoutePackagePath, Name: name, ResultIndex: 0}
}

func returnRouteMethod(name string) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: returnRoutePackagePath, Name: name,
		Receiver: goType(returnRoutePackagePath, "Route"), ResultIndex: -1,
	}
}

// returnRouteCarriers are the member set this rule selects over: one first
// return member of a body this call reaches, and the tag it is correlated by.
func returnRouteCarriers() []definition.Carrier {
	return []definition.Carrier{
		{Name: "BodyReturnRouteCarrier", Key: "carrier/value/body-return-route", Type: goType(returnRoutePackagePath, "Route")},
		{Name: "BodyReturnRouteTagCarrier", Key: "carrier/value/body-return-route-tag", Type: definition.GoType{Name: "uint64"}},
	}
}

// returnRoutes are the first return members this call site observes, derived
// once per invocation from the slot and the Call fact read at it. It is a
// relation rather than a table sealed at bind because WHICH bodies a call
// reaches is a property of that call's own dispatch, not of the binding.
func returnRoutes() definition.Relation {
	return definition.Relation{
		Name:    "BodyReturnRoutes",
		Key:     "value/body-result/routes",
		Subject: "BodyReturnRouteCarrier",
		Inputs: []definition.RelationInput{
			{Carrier: "MountedCallResultSlotCarrier"},
			{Carrier: "CallFactCarrier"},
		},
		CandidateProvider: valuebase.MountedCallResultSlotProvider(),
		Derivation: definition.RelationDerivation{
			State:      goType(returnRoutePackagePath, "Plan"),
			Build:      returnRouteFunction("Derive"),
			Count:      returnRouteFunction("Count"),
			At:         returnRouteFunction("At"),
			StaticAxes: []schema.EntryReference{axisReference("value"), axisReference("call")},
		},
	}
}

// Contribution is the body-result rule's whole share of the member
// vocabulary: the return route set it selects over and the fold that answers
// it. The result-zero directory it is indexed by and the coordinate it
// publishes at are the axis owner's, stated in the value base, because the
// result-alias rule reads the same two.
func Contribution() definition.Contribution {
	value := axisReference("value")
	call := axisReference("call")
	return definition.Contribution{
		Axis:      "value",
		Rule:      "value-callresult-body",
		Carriers:  append([]definition.Carrier{valuebase.MountedCallResultSlotCarrier(), valuebase.CallFactCarrier()}, returnRouteCarriers()...),
		Relations: []definition.Relation{valuebase.CallResultSites(), returnRoutes()},
		Projections: []definition.Projection{
			valuebase.CallResultSiteKey(),
			{
				Name: "BodyReturnRouteKey", Key: "value/body-result/route-key",
				Relation: "BodyReturnRoutes", CandidateProvider: valuebase.MountedCallResultSlotProvider(),
				Role: member.Key, Result: "ValueCoordinateCarrier", Accessor: returnRouteMethod("Coordinate"),
			},
			{
				Name: "BodyReturnRouteTag", Key: "value/body-result/route-tag",
				Relation: "BodyReturnRoutes", CandidateProvider: valuebase.MountedCallResultSlotProvider(),
				Role: member.Predicate, Result: "BodyReturnRouteTagCarrier", Accessor: returnRouteMethod("Predicate"),
			},
		},
		// The routes this rule reads are computed from the cells the reads
		// before them delivered, so they are published through this
		// selection and stamped with the tag the reading rule joins on.
		Selections: []definition.Selection{{
			Name:     "BodyReturnRouteSelection",
			Key:      "value/body-result/route-selection",
			Relation: "BodyReturnRoutes",
			Tag:      "BodyReturnRouteTag",
		}},
		Reducers: []definition.Reducer{{
			Name:      "BodyResultReducer",
			Key:       "value/body-result/reducer",
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
					Tag:          "BodyReturnRouteTagCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{Axis: value, Carrier: "ValueFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State:      goType(bodyResultPackagePath, "Judgment"),
				Build:      definition.GoSymbol{PackagePath: bodyResultPackagePath, Name: "Derive", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{axisReference("value"), axisReference("call")},
			},
			Implementation: definition.GoSymbol{
				PackagePath: bodyResultPackagePath, Name: "Result",
				Receiver: goType(bodyResultPackagePath, "Judgment"), ResultIndex: 0,
			},
		}},
	}
}
