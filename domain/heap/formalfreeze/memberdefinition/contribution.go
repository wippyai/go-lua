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
// Value owns and publishes the neutral mounted-call parent/member rows. This
// contribution declares only Heap's derived route set and reducer; it names
// Value's rows through the rule Program rather than restating them here.
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
			{Name: "FormalFreezeRouteTagCarrier", Key: "carrier/heap/formal-freeze-route-tag", Type: definition.GoType{Name: "uint64"}},
		},
		Relations: []definition.Relation{
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
					// The call's actuals arrive as the whole vector Value
					// publishes for the call, which is what join 1 declares and
					// what a member-set reader delivers. Naming it a selection
					// described a delivery its reader does not perform.
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
				},
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
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
				Name:              "FormalFreezeRouteKey",
				Key:               "heap/formal-freeze/route-key",
				Relation:          "FormalFreezeRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Key,
				Result:            "HeapKeyCarrier",
				Accessor:          method(recentPlanPackagePath, "Coordinates", recentPlanPackagePath, "Route", false, 0),
			},
			{
				// The route coordinate a member is published at. A routed
				// output publishes at the members a selection observed, so the
				// emitted worker pairs a cell with its member by this tag; the
				// tag is the one Heap already issued and this projection is
				// where the route set states it.
				Name:              "FormalFreezeRouteTag",
				Key:               "heap/formal-freeze/route-tag",
				Relation:          "FormalFreezeRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Predicate,
				Result:            "FormalFreezeRouteTagCarrier",
				Accessor: definition.GoSymbol{
					PackagePath: recentPlanPackagePath,
					Name:        "Predicate",
					Receiver:    goType(recentPlanPackagePath, "Route"),
					ResultIndex: -1,
				},
			},
			{
				// A freeze publishes back into the very root it read, so the
				// destination is the same coordinate under its own declared role.
				Name:              "FormalFreezeRouteDestination",
				Key:               "heap/formal-freeze/route-destination",
				Relation:          "FormalFreezeRoutes",
				CandidateProvider: member.AxisRelationCandidate(mountedCallProvider()),
				Role:              member.Destination,
				Result:            "HeapKeyCarrier",
				Accessor:          method(recentPlanPackagePath, "Coordinates", recentPlanPackagePath, "Route", false, 1),
			},
		},
		// The routes this rule reads are computed from the actuals the reads
		// before it delivered, so they are published by the derivation this
		// package already owns rather than enumerated from a directory.
		Selections: []definition.Selection{{
			Name:     "FormalFreezeRouteSelection",
			Key:      "heap/formal-freeze/route-selection",
			Relation: "FormalFreezeRoutes",
			Tag:      "FormalFreezeRouteTag",
		}},
		Reducers: []definition.Reducer{{
			Name: "FormalFreezeReducer",
			Key:  "heap/reducer/formal-freeze",
			Inputs: []definition.ReducerInput{{
				Axis:         heapAxis(),
				Carrier:      "HeapFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				// A routed form hands a member reducer the owner-issued tag its
				// cells were paired by. The coordinate is recovered from the tag
				// inside the judgment, by the schema that issued it.
				Tag: "FormalFreezeRouteTagCarrier",
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "FormalFreezeFact", ResultIndex: 0},
		}},
	}
}
