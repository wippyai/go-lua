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
	valuebase "github.com/wippyai/go-lua/domain/value/memberdefinition"
)

const (
	resultAliasPackagePath = "github.com/wippyai/go-lua/domain/value/resultalias"
	aliasRoutePackagePath  = "github.com/wippyai/go-lua/domain/value/resultalias/aliasroute"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
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
		CandidateProvider: valuebase.MountedCallResultSlotProvider(),
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
// vocabulary: the Call-side read it is addressed through, the alias route set
// it selects over, and the fold that answers them. The result-zero directory
// it is indexed by and the coordinate it publishes at are the axis owner's,
// stated in the value base, because the body-result rule reads the same two.
func Contribution() definition.Contribution {
	value := axisReference("value")
	call := axisReference("call")
	return definition.Contribution{
		Axis:      "value",
		Rule:      "value-callresult-resultalias",
		Carriers:  append([]definition.Carrier{valuebase.MountedCallResultSlotCarrier(), valuebase.CallFactCarrier()}, aliasRouteCarriers()...),
		Relations: []definition.Relation{valuebase.CallResultSites(), aliasRoutes()},
		Projections: []definition.Projection{
			valuebase.CallResultSiteKey(),
			{
				Name: "ResultAliasRouteKey", Key: "value/result-alias/route-key",
				Relation: "ResultAliasRoutes", CandidateProvider: valuebase.MountedCallResultSlotProvider(),
				Role: member.Key, Result: "ValueCoordinateCarrier", Accessor: aliasRouteMethod("Coordinate"),
			},
			{
				Name: "ResultAliasRouteTag", Key: "value/result-alias/route-tag",
				Relation: "ResultAliasRoutes", CandidateProvider: valuebase.MountedCallResultSlotProvider(),
				Role: member.Predicate, Result: "ResultAliasRouteTagCarrier", Accessor: aliasRouteMethod("Predicate"),
			},
		},
		// The routes this rule reads are computed from the cells the reads
		// before them delivered, so they are published through this
		// selection and stamped with the tag the reading rule joins on.
		Selections: []definition.Selection{{
			Name:     "ResultAliasRouteSelection",
			Key:      "value/result-alias/route-selection",
			Relation: "ResultAliasRoutes",
			Tag:      "ResultAliasRouteTag",
		}},
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
