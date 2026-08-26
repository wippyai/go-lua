// Package memberdefinition is the generator-only source for Suspension's
// owner-derived source/route relations and its scalar Placement reducer.
package memberdefinition

import (
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	suspensionPackagePath = "github.com/wippyai/go-lua/domain/placement/suspension"
	lifecyclePackagePath  = "github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
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

func contributionCarriers() ([]definition.Carrier, []definition.CarrierReference) {
	return []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key"), Capability: carrier.Equatable},
			{Name: "SuspensionRouteTagCarrier", Key: "carrier/placement/suspension-route-tag", Type: builtin("uint64"), Capability: carrier.DecodeOnly},
			{Name: "SourceSummaryCarrier", Key: "carrier/placement/suspension-source-summary", Type: goType(suspensionPackagePath, "SourceSummary"), Capability: carrier.DecodeOnly},
			{Name: "PlacementFactCarrier", Key: "carrier/placement/fact", Type: goType(placementPackagePath, "Fact"), Capability: carrier.Ascending},
			{Name: "SuspensionSourceCarrier", Key: "carrier/value/suspension-source", Type: goType(suspensionPackagePath, "Source"), Capability: carrier.DecodeOnly},
			{Name: "SuspensionRouteCarrier", Key: "carrier/placement/suspension-route", Type: goType(suspensionPackagePath, "Route"), Capability: carrier.DecodeOnly},
		}, []definition.CarrierReference{
			{Name: "SubjectLivenessCarrier", Key: "carrier/program/subject-liveness", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: programissuance.TypeSubjectLiveness}, Carrier: "carrier/program/subject-liveness"}, Type: goType(lifecyclePackagePath, "MountedSubjectLiveness")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Carrier: "carrier/value/coordinate"}, Type: goType(valuePackagePath, "Coordinate")},
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Carrier: "carrier/value/fact"}, Type: goType(valuePackagePath, "Value")},
		}
}

// provider is the candidate authority every dependent row of this rule hangs
// off: Program's authenticated subject-liveness row for this occurrence.
func provider() member.CandidateRef {
	return member.IssuedRowCandidate(programissuance.RelationOccurrenceSubjectLiveness)
}

func suspensionFunction(name string) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: suspensionPackagePath, Name: name, ResultIndex: 0}
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
			// The anchor is the Value owner's exact coordinate vector. It is
			// declared beside the source/route rows so the Program's first join
			// resolves an issued key column at the owner root.
			Name: "SuspensionAnchors", Key: "value/suspension/anchors", Axis: "value",
			Subject: "SuspensionSourceCarrier", Inputs: []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}},
			CandidateProvider: provider(),
			Addressing:        member.Addressing{Address: "value/suspension/anchor-key"},
		},
		{
			Name: "SuspensionSources", Key: "value/suspension/sources", Axis: "value",
			Subject:           "SuspensionSourceCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "SubjectLivenessCarrier"}},
			CandidateProvider: provider(),
			// The source owner, rather than a reader, states both columns by
			// which one produced source row is correlated.
			Addressing: member.Addressing{
				Address: "value/suspension/source-key",
				Tag:     "value/suspension/source-tag",
			},
		},
		{
			Name: "SuspensionRoutes", Key: "placement/suspension/routes",
			Subject: "SuspensionRouteCarrier",
			// Route derivation is the only Value-vector consumer. Its complete
			// delivery is summarized exactly once and retained on every route.
			Inputs: []definition.RelationInput{
				{Carrier: "SubjectLivenessCarrier"},
				{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormComplete},
			},
			CandidateProvider: provider(),
			// The route owner issues the candidate correlation and selection tag.
			// Destination remains the separate ordinary projection that a routed
			// publication names; it must never be fabricated from an address.
			Addressing: member.Addressing{
				Address: "placement/suspension/route-key",
				Tag:     "placement/suspension/route-tag",
			},
		},
	}
}

func producedProjections() []definition.Projection {
	return []definition.Projection{
		{
			Name: "SuspensionAnchorKey", Key: "value/suspension/anchor-key", Axis: "value",
			Relation: "SuspensionAnchors", Role: member.Key, Result: "ValueCoordinateCarrier",
			Accessor: sourceMethod("Coordinate", 0), CandidateProvider: provider(),
		},
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
		{
			Name: "SuspensionRouteSourceSummary", Key: "placement/suspension/route-source-summary",
			Relation: "SuspensionRoutes", Role: member.Attribute, Result: "SourceSummaryCarrier",
			Accessor: routeMethod("SourceSummary", 0), CandidateProvider: provider(),
		},
	}
}

func producedSelections() []definition.Selection {
	return []definition.Selection{
		{
			Name: "SuspensionSourceSelection", Key: "value/suspension/source-selection",
			Relation: "SuspensionSources", Tag: "SuspensionSourceTag",
			Implementation: suspensionFunction("DeriveSuspensionSources"),
		},
		{
			Name: "SuspensionRouteSelection", Key: "placement/suspension/route-selection",
			Relation: "SuspensionRoutes", Tag: "SuspensionRouteTag",
			Implementation: suspensionFunction("DeriveSuspensionRoutes"),
		},
	}
}

// Contribution is the Placement class reducer signature. The selected route
// retains all four scalar arguments: its owner-issued source summary, key,
// tag, and routed Placement cell. The fold never sees the Value vector that
// produced the route.
func Contribution() definition.Contribution {
	placementAxis := axisReference("placement")
	carriers, references := contributionCarriers()
	return definition.Contribution{
		Axis:     "placement",
		Rule:     "placement-suspension",
		Carriers: carriers, CarrierRefs: references,
		Relations:   producedRelations(),
		Projections: producedProjections(),
		Selections:  producedSelections(),
		Reducers: []definition.Reducer{{
			Name:      "SuspensionReducer",
			Key:       "placement/suspension/reducer",
			Candidate: "SubjectLivenessCarrier",
			Inputs: []definition.ReducerInput{
				{Axis: placementAxis, Carrier: "SourceSummaryCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: placementAxis, Carrier: "PlacementKeyCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: placementAxis, Carrier: "SuspensionRouteTagCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: placementAxis, Carrier: "PlacementFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier", Route: "PlacementKeyCarrier"},
			},
			Outputs:        []definition.ReducerOutput{{Axis: placementAxis, Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: suspensionPackagePath, Name: "SuspensionFold", ResultIndex: 0},
		}},
	}
}
