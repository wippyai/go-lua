// Package memberdefinition is the generator-only contribution of the
// suspension-evidence rule. It consumes the canonical Suspension route rows
// without claiming their source/vector or route-producing authority, and
// writes the evidence cell alone.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

const (
	suspensionPackagePath = "github.com/wippyai/go-lua/domain/placement/suspension"
	lifecyclePackagePath  = "github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	heapPackagePath       = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func goType(packagePath, name string) definition.GoType {
	return definition.GoType{PackagePath: packagePath, Name: name}
}

func builtin(name string) definition.GoType { return definition.GoType{Name: name} }

// contributionCarriers contains only the evidence fold's values. In
// particular it does not restate Source/Route subjects or their Value-vector
// inputs: those rows are owned once by the canonical Placement suspension
// route relation.
func contributionCarriers() ([]definition.Carrier, []definition.CarrierReference) {
	return []definition.Carrier{
			{Name: "PlacementKeyCarrier", Key: "carrier/placement/key", Type: goType(heapPackagePath, "Key"), Capability: carrier.Equatable},
			{Name: "SourceSummaryCarrier", Key: "carrier/placement/suspension-source-summary", Type: goType(suspensionPackagePath, "SourceSummary"), Capability: carrier.DecodeOnly},
			{Name: "SuspensionRouteTagCarrier", Key: "carrier/placement/suspension-route-tag", Type: builtin("uint64"), Capability: carrier.DecodeOnly},
			{Name: "EvidenceFactCarrier", Key: "carrier/placement/suspension-evidence/fact", Type: goType(suspensionPackagePath, "Evidence"), Capability: carrier.Ascending},
		}, []definition.CarrierReference{
			{Name: "SubjectLivenessCarrier", Key: "carrier/program/subject-liveness", Ref: carrier.Ref{Owner: schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: programissuance.TypeSubjectLiveness}, Carrier: "carrier/program/subject-liveness"}, Type: goType(lifecyclePackagePath, "MountedSubjectLiveness")},
		}
}

// Contribution is the independent evidence reducer signature. It has no
// relation/projection/selection rows: Selection's presence is the sole
// production authority, and canonical SuspensionRoutes already declares it.
func Contribution() definition.Contribution {
	evidenceAxis := axisReference("placement-suspension-evidence")
	carriers, references := contributionCarriers()
	return definition.Contribution{
		Axis:     "placement-suspension-evidence",
		Rule:     "placement-suspension-evidence",
		Carriers: carriers, CarrierRefs: references,
		Reducers: []definition.Reducer{{
			Name:      "SuspensionEvidenceReducer",
			Key:       "placement-suspension-evidence/reducer",
			Candidate: "SubjectLivenessCarrier",
			Inputs: []definition.ReducerInput{
				{Axis: evidenceAxis, Carrier: "SourceSummaryCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: evidenceAxis, Carrier: "PlacementKeyCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: evidenceAxis, Carrier: "SuspensionRouteTagCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier"},
				{Axis: evidenceAxis, Carrier: "EvidenceFactCarrier", Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: "SuspensionRouteTagCarrier", Route: "PlacementKeyCarrier"},
			},
			Outputs:        []definition.ReducerOutput{{Axis: evidenceAxis, Carrier: "EvidenceFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: suspensionPackagePath, Name: "SuspensionEvidenceFold", ResultIndex: 0},
		}},
	}
}
