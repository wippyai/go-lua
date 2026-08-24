// Package memberdefinition is the generator-only owner source for the Heap
// formal-freeze rule's own relation, projections and reducer. It is imported
// by the member definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	heapPackagePath       = "github.com/wippyai/go-lua/domain/heap"
	callPackagePath       = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath      = "github.com/wippyai/go-lua/domain/value"
	freezePackagePath     = "github.com/wippyai/go-lua/domain/heap/formalfreeze"
	recentPlanPackagePath = "github.com/wippyai/go-lua/domain/heap/internal/recentplan"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func heapAxis() schema.EntryReference { return axisReference("heap") }

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func builtinGoType(name string) definition.GoType { return definition.GoType{Name: name} }

func method(path, name, receiverPath, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     path,
		Name:            name,
		Receiver:        goType(receiverPath, receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func freezeFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: freezePackagePath, Name: name, ResultIndex: 0}
}

// mountedCallProvider is the foreign candidate directory every row this rule
// declares hangs off: Call owns which mounted calls exist, and no axis here
// mirrors that directory locally.
func mountedCallProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("call"), Member: "call/mounted-call/candidates"}
}

func actualMemberProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("value"), Member: "value/formal-freeze/actual-members"}
}

func callActualsProvider() member.RelationRef {
	return member.RelationRef{Axis: axisReference("value"), Member: "value/formal-freeze/call-actuals"}
}

// Contribution is the Heap formal-freeze rule's own member declaration.
//
// The rule reads three things and folds one. Its candidate is a mounted call.
// Join zero is that call's own fact, read exactly at the coordinate the call
// axis already projects for the candidate. Join one is the call's ordered
// mounted actuals - a nested member set, so which actuals a call has is a
// MEMBERSHIP statement Value publishes by grouping rows it already sealed,
// not a correlation this rule recomputes from Pack geometry per invocation.
// Join two is the freeze route set derived from those two, and it is the only
// row here whose construction is still authored.
//
// The rows are declared per axis because a relation over Value coordinates is
// Value's data whichever rule needs it, and the roster folds each row into the
// source of the axis it names.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-formal-freeze",
		Carriers: []definition.Carrier{
			// The two Call carriers this rule's route relation is typed in. They
			// repeat Call's own declaration verbatim; composition refuses a
			// repeat that disagrees.
			{Name: "CallCoordinateCarrier", Key: "carrier/call/mounted-call", Type: goType(callPackagePath, "CallCoordinate")},
			{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
			{Name: "FormalFreezeRouteCarrier", Key: "carrier/heap/formal-freeze-route", Type: goType(recentPlanPackagePath, "Route")},
			// Value-side carriers, carried to the value source with the rows
			// they type.
			{Name: "MountedCallActualsCarrier", Key: "carrier/value/mounted-call-actuals", Type: goType(valuePackagePath, "MountedCallActuals")},
			{Name: "MountedCallActualTagCarrier", Key: "carrier/value/mounted-call-actual-tag", Type: builtinGoType("uint64")},
		},
		Relations: []definition.Relation{
			{
				// The mounted call's own fact, read at the coordinate Call
				// already projects for the candidate. It is a dependent relation
				// over the candidate row rather than the directory itself: a
				// join reads rows a relation derives FROM something, and what
				// this derives from is the mounted call it is handed.
				Axis:              "call",
				Name:              "MountedCallFacts",
				Key:               "call/mounted-call/facts",
				Subject:           "CallFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: mountedCallProvider(),
			},
			{
				// The parent of the actual member set: one row per mounted call,
				// resolved under the same occurrence the mounted call candidate
				// is. It is Value grouping its own sealed actual rows by the
				// (module, call) prefix of their key, so it publishes no call
				// identity of its own.
				Axis:              "value",
				Name:              "FormalFreezeCallActuals",
				Key:               "value/formal-freeze/call-actuals",
				Subject:           "MountedCallActualsCarrier",
				CandidateProvider: callActualsProvider(),
				CandidateResolver: method(valuePackagePath, "MountedCallActualsForMountedOccurrence", valuePackagePath, "Schema", true, 0),
				CandidateOrdinal:  method(valuePackagePath, "MountedCallActualsOrdinal", valuePackagePath, "Schema", true, 0),
				CandidateAt:       method(valuePackagePath, "MountedCallActualsAt", valuePackagePath, "Schema", true, 0),
			},
			{
				// The ordered actuals themselves, addressed by (call, ordinal).
				// The set is self-provided, so a member densifies through this
				// relation's own directory and projects the way every other row
				// of it does.
				Axis:              "value",
				Name:              "FormalFreezeActualMembers",
				Key:               "value/formal-freeze/actual-members",
				Subject:           "MountedCallArgumentCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "CallCoordinateCarrier"}},
				CandidateProvider: actualMemberProvider(),
				CandidateResolver: method(valuePackagePath, "MountedCallArgumentForMountedOccurrence", valuePackagePath, "Schema", true, 0),
				CandidateOrdinal:  method(valuePackagePath, "MountedCallArgumentOrdinal", valuePackagePath, "Schema", true, 0),
				CandidateAt:       method(valuePackagePath, "MountedCallArgumentAt", valuePackagePath, "Schema", true, 0),
				MemberParent:      callActualsProvider(),
				MemberOrdinal:     "MountedCallActualTagCarrier",
				MemberCount:       method(valuePackagePath, "MemberCount", valuePackagePath, "MountedCallActuals", false, 0),
				MemberAt:          method(valuePackagePath, "MemberAt", valuePackagePath, "MountedCallActuals", false, 0),
			},
			{
				// The freeze route set: the exact Recent allocation roots the
				// known targets of this call all justify freezing. Its inputs are
				// the candidate, the call fact, and the ordered cells the actual
				// selection answered - a route set computed from every actual
				// cannot be handed one actual at a time.
				Name:    "FormalFreezeRoutes",
				Key:     "heap/formal-freeze/routes",
				Subject: "FormalFreezeRouteCarrier",
				Inputs: []definition.RelationInput{
					{Carrier: "CallCoordinateCarrier"},
					{Carrier: "CallFactCarrier"},
					{Carrier: "ValueFactCarrier", Many: true},
				},
				CandidateProvider: mountedCallProvider(),
				Derivation: definition.RelationDerivation{
					State: goType(recentPlanPackagePath, "Plan"),
					Build: freezeFunction("DeriveFreezeRoutes"),
					Count: freezeFunction("FreezeRouteCount"),
					At:    freezeFunction("FreezeRouteAt"),
					StaticAxes: []schema.EntryReference{
						heapAxis(),
						axisReference("value"),
						axisReference("call"),
						axisReference("pack"),
					},
				},
			},
		},
		Projections: []definition.Projection{
			{
				// The coordinate the call fact is read at. The accessor's
				// receiver is the relation's input - the mounted call row - which
				// is what a dependent relation keyed on its own candidate means.
				Axis:              "call",
				Name:              "MountedCallFactKey",
				Key:               "call/mounted-call/fact-key",
				Relation:          "MountedCallFacts",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Key,
				Result:            "CallKeyCarrier",
				Accessor:          method(callPackagePath, "Key", callPackagePath, "CallCoordinate", false, -1),
			},
			{
				Axis:              "value",
				Name:              "FormalFreezeActualKey",
				Key:               "value/formal-freeze/actual-key",
				Relation:          "FormalFreezeActualMembers",
				CandidateProvider: actualMemberProvider(),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          method(valuePackagePath, "Coordinate", valuePackagePath, "MountedCallArgument", false, -1),
			},
			{
				// The selection tag is the owner-issued one-based address of the
				// actual under its call, so the rule selects a member by the tag
				// Value published rather than by a convention of its own.
				Axis:              "value",
				Name:              "FormalFreezeActualTag",
				Key:               "value/formal-freeze/actual-tag",
				Relation:          "FormalFreezeActualMembers",
				CandidateProvider: actualMemberProvider(),
				Role:              member.Predicate,
				Result:            "MountedCallActualTagCarrier",
				Accessor:          method(valuePackagePath, "ActualTag", valuePackagePath, "MountedCallArgument", false, -1),
			},
			{
				Name:              "FormalFreezeRouteKey",
				Key:               "heap/formal-freeze/route-key",
				Relation:          "FormalFreezeRoutes",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Key,
				Result:            "HeapKeyCarrier",
				Accessor:          method(recentPlanPackagePath, "Coordinates", recentPlanPackagePath, "Route", false, 0),
			},
			{
				// A freeze publishes back into the very root it read, so the
				// destination is the same coordinate under its own declared role.
				Name:              "FormalFreezeRouteDestination",
				Key:               "heap/formal-freeze/route-destination",
				Relation:          "FormalFreezeRoutes",
				CandidateProvider: mountedCallProvider(),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          method(recentPlanPackagePath, "Coordinates", recentPlanPackagePath, "Route", false, 1),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "FormalFreezeReducer",
			Key:  "heap/reducer/formal-freeze",
			Inputs: []definition.ReducerInput{{
				Axis:         heapAxis(),
				Carrier:      "HeapFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Route:        "HeapKeyCarrier",
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "FormalFreezeFact", ResultIndex: 0},
		}},
	}
}
