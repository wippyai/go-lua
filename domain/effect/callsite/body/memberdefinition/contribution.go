// Package memberdefinition is the generator-only owner source for the
// interprocedural Call-to-Effect rule's own fold. It is imported by the member
// definition roster and by nothing at runtime, so the callsite package keeps
// its judgment and none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	effectbase "github.com/wippyai/go-lua/domain/effect/memberdefinition"
)

const (
	callsitePackagePath  = "github.com/wippyai/go-lua/domain/effect/callsite"
	bodyRoutePackagePath = "github.com/wippyai/go-lua/domain/effect/callsite/bodyroute"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// bodyRouteCarriers are the member set the interprocedural rule selects over:
// one body this call dispatches to, and the tag its root is correlated by.
func bodyRouteCarriers() []definition.Carrier {
	return []definition.Carrier{
		{Name: "BodyRouteCarrier", Key: "carrier/effect/body-route", Type: definition.GoType{PackagePath: bodyRoutePackagePath, Name: "Route"}},
		{Name: "BodyRouteTagCarrier", Key: "carrier/effect/body-route-tag", Type: definition.GoType{Name: "uint64"}},
	}
}

func bodyRouteMethod(name string) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: bodyRoutePackagePath, Name: name,
		Receiver:    definition.GoType{PackagePath: bodyRoutePackagePath, Name: "Route"},
		ResultIndex: -1,
	}
}

func bodyRouteFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: bodyRoutePackagePath, Name: name, ResultIndex: 0}
}

func mountedCallProvider() member.CandidateRef {
	return member.AxisRelationCandidate(member.RelationRef{
		Axis: axisReference("effect"), Member: "effect/mounted-call/candidates",
	})
}

// bodyRoutes is the bodies this call site reaches, derived once per invocation
// from the site and the Call fact read at it. It is a relation rather than a
// table sealed at bind because WHICH bodies a call reaches is a property of
// that call's own dispatch, not of the binding.
func bodyRoutes() definition.Relation {
	return definition.Relation{
		Name: "BodyRoutes", Key: "effect/callsite/body-routes",
		Subject: "BodyRouteCarrier",
		Inputs: []definition.RelationInput{
			{Carrier: "EffectMountedCallCarrier"},
			{Carrier: "CallFactCarrier"},
		},
		CandidateProvider: mountedCallProvider(),
		// Declared rather than authored: what to enumerate, how to union it,
		// what to widen to when the call names no alternatives, and the order
		// the members come back in are all stated here and written by the
		// emitter. The one authored symbol left is the judgment that says what
		// a single call target means to Effect.
		Derivation: definition.RelationDerivation{
			StaticAxes: []schema.EntryReference{axisReference("effect"), axisReference("call")},
			Source:     []definition.EnumerationRef{{Axis: axisReference("call"), Name: "KnownTargets"}},
			Resolve:    bodyRouteFunction("ResolveRoute"),
			// A call site reaches one body when its target is known and a
			// handful when its dispatch is over a closed set, so the ordinary
			// answer is held by value and never allocates.
			InlineWidth: 4,
			Widen: definition.DerivationWiden{
				// A call value that named no alternatives reaches every body
				// there is, and only Call's own directory can say which those
				// are.
				Predicate: bodyRouteFunction("BeyondTargets"),
				Source:    []definition.EnumerationRef{{Axis: axisReference("call"), Name: "BodyTargets"}},
			},
		},
	}
}

// Contribution is the interprocedural reading's fold: the effect of every
// executable body this call reaches, transported to the site and joined.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:     "effect",
		Rule:     "effect-body",
		Carriers: append([]definition.Carrier{effectbase.CallFactCarrier(), effectbase.MountedCallCarrier()}, bodyRouteCarriers()...),
		Relations: []definition.Relation{
			effectbase.EffectSites(),
			bodyRoutes(),
		},
		Projections: []definition.Projection{
			effectbase.EffectSiteKey(),
			{
				Name: "BodyRouteKey", Key: "effect/callsite/body-route-key",
				Relation: "BodyRoutes", CandidateProvider: mountedCallProvider(),
				Role: member.Key, Result: "EffectKeyCarrier", Accessor: bodyRouteMethod("Coordinate"),
			},
			{
				Name: "BodyRouteTag", Key: "effect/callsite/body-route-tag",
				Relation: "BodyRoutes", CandidateProvider: mountedCallProvider(),
				Role: member.Predicate, Result: "BodyRouteTagCarrier", Accessor: bodyRouteMethod("Predicate"),
			},
		},
		// The body routes are computed from the call fact the read before them
		// delivered, so they are published through this selection and stamped
		// with the tag the reading rule joins on.
		Selections: []definition.Selection{{
			Name:     "BodyRouteSelection",
			Key:      "effect/callsite/body-route-selection",
			Relation: "BodyRoutes",
			Tag:      "BodyRouteTag",
		}},
		Reducers: []definition.Reducer{{
			Name:      "BodyCallEffectReducer",
			Key:       "effect/callsite-body/reducer",
			Candidate: "EffectMountedCallCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:    axisReference("effect"),
				Carrier: "EffectFactCarrier",
				Form:    member.ReadFormSelected,
				// Many-valued: this fold is handed the whole selection and
				// concludes once over it. The tag carrier is still named,
				// because which carrier names a member is the joined axis's
				// statement either way - a delivery this wide carries the tags
				// inside its cells rather than as an argument beside them.
				Multiplicity: member.MultiplicityMany,
				Tag:          "BodyRouteTagCarrier",
			}},
			Outputs: []definition.ReducerOutput{{Axis: axisReference("effect"), Carrier: "EffectFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State:      effectbase.CallsiteJudgmentType(),
				Build:      definition.GoSymbol{PackagePath: callsitePackagePath, Name: "DeriveBody", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{axisReference("effect"), axisReference("call")},
			},
			Implementation: definition.GoSymbol{
				PackagePath: callsitePackagePath, Name: "BodyEffect",
				Receiver: effectbase.CallsiteJudgmentType(), ResultIndex: 0,
			},
		}},
	}
}
