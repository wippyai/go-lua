// Package memberdefinition is the generator-only source for the two
// suspension reducers.  It contributes reducer signatures, not runtime
// binders or route planners.  The shared Value source relation and the two
// route relation/output rows are intentionally left for the FT-25 owner seam:
// their current owner accessors are not exported from this family yet.
package memberdefinition

import (
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	suspensionPackagePath = "github.com/wippyai/go-lua/domain/placement/suspension"
	lifecyclePackagePath  = "github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	callPackagePath       = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath      = "github.com/wippyai/go-lua/domain/value"
	placementPackagePath  = "github.com/wippyai/go-lua/domain/placement"
	heapPackagePath       = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func builtin(name string) definition.GoType { return definition.GoType{Name: name} }

func contributionCarriers(includeEvidence bool) []definition.Carrier {
	carriers := []definition.Carrier{
		{Name: "SubjectLivenessCarrier", Key: "carrier/program/subject-liveness", Type: goType(lifecyclePackagePath, "MountedSubjectLiveness")},
		{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: goType(valuePackagePath, "Coordinate")},
		{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
		{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
		{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
		{Name: "SuspensionRouteTagCarrier", Key: "carrier/placement/suspension-route-tag", Type: builtin("uint64")},
	}
	if includeEvidence {
		carriers = append(carriers,
			definition.Carrier{Name: "EvidenceFactCarrier", Key: "carrier/placement/suspension-evidence/fact", Type: goType(suspensionPackagePath, "Evidence")},
			definition.Carrier{Name: "SuspensionEvidenceRouteTagCarrier", Key: "carrier/placement/suspension-evidence-route-tag", Type: builtin("uint64")},
			definition.Carrier{Name: "SuspensionSourceCarrier", Key: "carrier/value/suspension-source", Type: goType(suspensionPackagePath, "Source")},
			definition.Carrier{Name: "SuspensionRouteCarrier", Key: "carrier/placement/suspension-route", Type: goType(suspensionPackagePath, "Route")},
		)
	} else {
		carriers = append(carriers,
			definition.Carrier{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact")},
			definition.Carrier{Name: "SuspensionSourceCarrier", Key: "carrier/value/suspension-source", Type: goType(suspensionPackagePath, "Source")},
			definition.Carrier{Name: "SuspensionRouteCarrier", Key: "carrier/placement/suspension-route", Type: goType(suspensionPackagePath, "Route")},
		)
	}
	return carriers
}

// provider is the candidate authority every dependent row of this rule hangs
// off: Program's authenticated subject-liveness row for this occurrence.
func provider() member.CandidateRef {
	return member.IssuedRowCandidate(programissuance.RelationOccurrenceSubjectLiveness)
}

func sourceMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: suspensionPackagePath, Name: name, Receiver: goType(suspensionPackagePath, "Source"), ResultIndex: result}
}

func routeMethod(name string, result int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: suspensionPackagePath, Name: name, Receiver: goType(suspensionPackagePath, "Route"), ResultIndex: result}
}

// producedRows is the suspension rule's own vector vocabulary: the two
// relations its Program reads, the projections they are addressed by, and the
// operations that publish them. Both vectors are produced - the cells are read
// out of the publication the candidate was redeemed from, and the allocation
// is resolved through the mounted call its boundary names - so neither is a
// coordinate anything could pair against until the operation has run.
func producedRelations() []definition.Relation {
	return []definition.Relation{
		{
			Name: "SuspensionSources", Key: "value/suspension/sources", Axis: "value",
			Subject:           "SuspensionSourceCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}},
			CandidateProvider: provider(),
		},
		{
			Name: "SuspensionRoutes", Key: "placement/suspension/routes",
			Subject: "SuspensionRouteCarrier",
			// The route set answers to the boundary call as well as to the
			// liveness row: a call whose every target operation declares only
			// a normal outcome is not a yield boundary and reaches no route.
			Inputs:            []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}, {Carrier: "CallFactCarrier"}},
			CandidateProvider: provider(),
		},
	}
}

func producedProjections() []definition.Projection {
	return []definition.Projection{
		{
			Name: "SuspensionSourceKey", Key: "value/suspension/source-key", Axis: "value",
			Relation: "SuspensionSources", Role: member.Key, Result: "ValueCoordinateCarrier",
			Accessor: sourceMethod("Coordinate", 0), CandidateProvider: provider(),
		},
		{
			Name: "SuspensionSourceTag", Key: "value/suspension/source-tag", Axis: "value",
			Relation: "SuspensionSources", Role: member.Predicate, Result: "SuspensionRouteTagCarrier",
			Accessor: sourceMethod("Tag", 0), CandidateProvider: provider(),
		},
		{
			Name: "SuspensionRouteKey", Key: "placement/suspension/route-key",
			Relation: "SuspensionRoutes", Role: member.Key, Result: "PlacementKeyCarrier",
			Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
		},
		{
			Name: "SuspensionRouteTag", Key: "placement/suspension/route-tag",
			Relation: "SuspensionRoutes", Role: member.Predicate, Result: "SuspensionRouteTagCarrier",
			Accessor: routeMethod("Predicate", 0), CandidateProvider: provider(),
		},
		{
			Name: "SuspensionRouteDestination", Key: "placement/suspension/route-destination",
			Relation: "SuspensionRoutes", Role: member.Destination, Result: "PlacementKeyCarrier",
			Accessor: routeMethod("Coordinates", 1), CandidateProvider: provider(),
		},
	}
}

func producedSelections() []definition.Selection {
	return []definition.Selection{
		{
			Name: "SuspensionSourceSelection", Key: "value/suspension/source-selection",
			Relation: "SuspensionSources", Tag: "SuspensionSourceTag",
		},
		{
			Name: "SuspensionRouteSelection", Key: "placement/suspension/route-selection",
			Relation: "SuspensionRoutes", Tag: "SuspensionRouteTag",
		},
	}
}

// Contribution is the Placement class reducer signature.  Its third input
// is the routed Placement cell; the route coordinate and tag are delivered by
// the selected route relation named by the Program, not reconstructed by this
// declaration.
func Contribution() definition.Contribution {
	valueAxis := axisReference("value")
	placementAxis := axisReference("placement")
	return definition.Contribution{
		Axis:        "placement",
		Rule:        "placement-suspension",
		Carriers:    contributionCarriers(false),
		Relations:   producedRelations(),
		Projections: producedProjections(),
		Selections:  producedSelections(),
		Reducers: []definition.Reducer{{
			Name:      "SuspensionReducer",
			Key:       "placement/suspension/reducer",
			Candidate: "SubjectLivenessCarrier",
			// The anchor read states the denominator the vector is complete
			// against, and a denominator is not a fold input: the fold takes
			// the vector and the routed cell, which is what the judgment takes.
			Inputs: []definition.ReducerInput{
				{Axis: valueAxis, Carrier: "ValueFactCarrier", Form: member.ReadFormSummary, Multiplicity: member.MultiplicityMany, Tag: "SuspensionRouteTagCarrier"},
				{Axis: placementAxis, Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier", Route: "PlacementKeyCarrier"},
			},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis, Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: suspensionPackagePath, Name: "SuspensionFold", ResultIndex: 0},
		}},
	}
}
