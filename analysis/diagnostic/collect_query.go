package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/conformance"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ConformanceSubject is one TypeConformance site: the value the program
// assigns or passes, the declaration it is measured against, and the base
// evidence points its measured value is observed at.
type ConformanceSubject struct {
	ID, FindingID, Mount identity.ContentID
	Location             DiagnosticLocation
	Site                 schemadiag.Site
	Actual               uint32
	DeclaredMay          runtimekind.Set
	Target               string
	// Points are the execution points of the occurrences that produce the
	// measured value. A statement carries one base evidence point and as many
	// producing occurrences as it has measured values, so a subject addresses
	// its own column by its producers rather than by the point they anchor to.
	Points []identity.ContentID
}

// CollectConformance reads each issued TypeConformance site against the Value
// summary observed at the rule occurrence that produced the measured value.
// The judgment is MayKindConformance: only a family outside the declaration's
// may-set becomes a finding. Empty, All, and unprojected declarations abstain.
//
// The measured value is read from the observation column, not from a
// point-keyed query column. A query site is published only at selected points,
// and the occurrence that produces an argument or initializer value is not one
// of them, so a query read abstains on every site by construction.
func CollectConformance(
	report *DiagnosticReport,
	subjects []ConformanceSubject,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	severities map[schemadiag.Site]FindingSeverity,
) bool {
	if report == nil || schema == nil || published == nil || !published.Published() || !observationPlan.Available() || valueWidth < 0 || len(severities) == 0 {
		return false
	}
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.FindingID.Available() || !subject.Mount.Available() || !subject.Location.Available() ||
			!subject.Site.Available() || !subject.DeclaredMay.Valid() || len(subject.Points) == 0 {
			return false
		}
		severity, enabled := severities[subject.Site]
		if !enabled || !severity.Available() {
			continue
		}
		if subject.DeclaredMay == runtimekind.All {
			continue
		}
		var observed runtimekind.Set
		anyEvidence := false
		for _, point := range subject.Points {
			publicationKey, addressed := ValueObservationAddress(structure.DiagnosticObservationTypeConformance, subject.Mount, point)
			if !point.Available() || !addressed {
				return false
			}
			summary, readable := PublishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, publicationKey)
			if !readable {
				report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
				return true
			}
			if !summary.Valid {
				report.SetCollectionFailure(DiagnosticCollectionQueryInvalid)
				return true
			}
			if len(summary.Values) != len(summary.Present) || len(summary.Values) != valueWidth || summary.Rows > 1 {
				report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
				return true
			}
			if summary.Rows == 0 {
				continue
			}
			index := int(subject.Actual)
			if index < 0 || index >= len(summary.Values) {
				report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
				return true
			}
			if !summary.Present[index] {
				continue
			}
			kinds := schema.RuntimeKinds(summary.Values[index])
			if !kinds.Valid() {
				return false
			}
			observed |= kinds
			anyEvidence = true
		}
		if !anyEvidence {
			continue
		}
		switch conformance.MayKindConformance(subject.DeclaredMay, observed) {
		case conformance.VerdictViolates:
			if !appendConformanceFinding(report, subject, severity) {
				return false
			}
		case conformance.VerdictConforms, conformance.VerdictAbstain:
		default:
			return false
		}
	}
	return true
}

func appendConformanceFinding(report *DiagnosticReport, subject ConformanceSubject, severity FindingSeverity) bool {
	if report == nil || !subject.ID.Available() || !subject.FindingID.Available() || !severity.Available() {
		return false
	}
	name, nameOK := NewSemanticName(conformanceSubjectName(subject.Site))
	target, targetOK := NewTargetType(subject.Target)
	if !nameOK || !targetOK || !subject.Location.Available() {
		return false
	}
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return false
	}
	declared, declaredOK := table.ForBranchObservation(structure.DiagnosticObservationTypeConformance.Key(), subject.Site)
	data := NewTemplateData(name, target, 0, DiagnosticLocation{})
	if !declaredOK || !declared.Site().Available() || !data.ValidFor(declared) {
		return false
	}
	report.AppendFinding(NewFindingRow(subject.FindingID, subject.ID, declared.Code(), severity, subject.Location, data))
	return true
}

func conformanceSubjectName(site schemadiag.Site) string {
	if site == schemadiag.SiteAssignment {
		return "value"
	}
	return "argument"
}
