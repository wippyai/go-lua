// Package memberdefinition is the generator-only contribution of the
// suspension-evidence rule: the two vectors it reads, the operations that
// publish them, and its fold signature. It writes the evidence cell alone and
// never Placement class, which is why it is a separate rule from the
// suspension consumer rather than a second output of it.
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
	heapPackagePath       = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func builtin(name string) definition.GoType { return definition.GoType{Name: name} }

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

// contributionCarriers is the evidence rule's own carrier vocabulary: the
// neutral candidate and Value rows it reads, and the evidence cell and route
// tag that are its alone.
func contributionCarriers() []definition.Carrier {
	return []definition.Carrier{
		{Name: "SubjectLivenessCarrier", Key: "carrier/program/subject-liveness", Type: goType(lifecyclePackagePath, "MountedSubjectLiveness")},
		{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: goType(valuePackagePath, "Coordinate")},
		{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
		{Name: "CallFactCarrier", Key: "carrier/call/fact", Type: goType(callPackagePath, "Value")},
		{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key")},
		{Name: "EvidenceFactCarrier", Key: "carrier/placement/suspension-evidence/fact", Type: goType(suspensionPackagePath, "Evidence")},
		{Name: "SuspensionEvidenceRouteTagCarrier", Key: "carrier/placement/suspension-evidence-route-tag", Type: builtin("uint64")},
		{Name: "SuspensionSourceCarrier", Key: "carrier/value/suspension-source", Type: goType(suspensionPackagePath, "Source")},
		{Name: "SuspensionRouteCarrier", Key: "carrier/placement/suspension-route", Type: goType(suspensionPackagePath, "Route")},
	}
}

// relations, projections and selections are the same
// two produced vectors stated for the independent evidence producer. It reads
// its own rows under its own keys - sharing the suspension consumer's would
// make one vector answer to two writers - and publishes them through the same
// owner judgments, which do not depend on which Factor consumes the rows.
func relations() []definition.Relation {
	return []definition.Relation{
		{
			Name: "EvidenceSources", Key: "value/suspension-evidence/sources", Axis: "value",
			Subject:           "SuspensionSourceCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}},
			CandidateProvider: provider(),
		},
		{
			Name: "EvidenceRoutes", Key: "placement/suspension-evidence/routes",
			Subject: "SuspensionRouteCarrier",
			// The route set answers to the boundary call as well as to the
			// liveness row: a call whose every target operation declares only
			// a normal outcome is not a yield boundary and reaches no route.
			Inputs:            []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}, {Carrier: "CallFactCarrier"}},
			CandidateProvider: provider(),
		},
	}
}

func projections() []definition.Projection {
	return []definition.Projection{
		{
			Name: "EvidenceSourceKey", Key: "value/suspension-evidence/source-key", Axis: "value",
			Relation: "EvidenceSources", Role: member.Key, Result: "ValueCoordinateCarrier",
			Accessor: sourceMethod("Coordinate", 0), CandidateProvider: provider(),
		},
		{
			Name: "EvidenceSourceTag", Key: "value/suspension-evidence/source-tag", Axis: "value",
			Relation: "EvidenceSources", Role: member.Predicate, Result: "SuspensionEvidenceRouteTagCarrier",
			Accessor: sourceMethod("Tag", 0), CandidateProvider: provider(),
		},
		{
			Name: "EvidenceRouteKey", Key: "placement/suspension-evidence/route-key",
			Relation: "EvidenceRoutes", Role: member.Key, Result: "PlacementKeyCarrier",
			Accessor: routeMethod("Coordinates", 0), CandidateProvider: provider(),
		},
		{
			Name: "EvidenceRouteTag", Key: "placement/suspension-evidence/route-tag",
			Relation: "EvidenceRoutes", Role: member.Predicate, Result: "SuspensionEvidenceRouteTagCarrier",
			Accessor: routeMethod("Predicate", 0), CandidateProvider: provider(),
		},
		{
			Name: "EvidenceRouteDestination", Key: "placement/suspension-evidence/route-destination",
			Relation: "EvidenceRoutes", Role: member.Destination, Result: "PlacementKeyCarrier",
			Accessor: routeMethod("Coordinates", 1), CandidateProvider: provider(),
		},
	}
}

func selections() []definition.Selection {
	return []definition.Selection{
		{
			Name: "EvidenceSourceSelection", Key: "value/suspension-evidence/source-selection",
			Relation: "EvidenceSources", Tag: "EvidenceSourceTag",
		},
		{
			Name: "EvidenceRouteSelection", Key: "placement/suspension-evidence/route-selection",
			Relation: "EvidenceRoutes", Tag: "EvidenceRouteTag",
		},
	}
}

// Contribution is the independent evidence reducer signature.  It
// repeats the neutral E/J inputs but names a different output axis, carrier,
// route tag, and implementation.  It never reads PlacementFactCarrier.
func Contribution() definition.Contribution {
	valueAxis := axisReference("value")
	evidenceAxis := axisReference("placement-suspension-evidence")
	return definition.Contribution{
		Axis:        "placement-suspension-evidence",
		Rule:        "placement-suspension-evidence",
		Carriers:    contributionCarriers(),
		Relations:   relations(),
		Projections: projections(),
		Selections:  selections(),
		Reducers: []definition.Reducer{{
			Name:      "SuspensionEvidenceReducer",
			Key:       "placement-suspension-evidence/reducer",
			Candidate: "SubjectLivenessCarrier",
			Inputs: []definition.ReducerInput{
				{Axis: valueAxis, Carrier: "ValueFactCarrier", Form: member.ReadFormSummary, Multiplicity: member.MultiplicityMany, Tag: "SuspensionEvidenceRouteTagCarrier"},
				{Axis: evidenceAxis, Carrier: "EvidenceFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionEvidenceRouteTagCarrier", Route: "PlacementKeyCarrier"},
			},
			Outputs:        []definition.ReducerOutput{{Axis: evidenceAxis, Carrier: "EvidenceFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: suspensionPackagePath, Name: "SuspensionEvidenceFold", ResultIndex: 0},
		}},
	}
}
